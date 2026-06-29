package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"bloodpressure/backend/internal/config"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

type Service struct {
	db           *sql.DB
	tokenManager *TokenManager
	refreshTTL   time.Duration
	accessTTL    time.Duration
}

func NewService(db *sql.DB, cfg config.Config) *Service {
	return &Service{
		db:           db,
		tokenManager: NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL),
		refreshTTL:   cfg.RefreshTokenTTL,
		accessTTL:    cfg.AccessTokenTTL,
	}
}

func (s *Service) Register(ctx context.Context, email, password string) (TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(password) < 8 {
		return TokenPair{}, ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return TokenPair{}, fmt.Errorf("hash password: %w", err)
	}

	userID := newUUID()
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users(id, email, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
	`, userID, email, string(hash), now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return TokenPair{}, ErrEmailTaken
		}
		return TokenPair{}, fmt.Errorf("insert user: %w", err)
	}

	return s.issueTokens(ctx, userID)
}

func (s *Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var userID, passwordHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, password_hash FROM users WHERE email = $1
	`, email).Scan(&userID, &passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TokenPair{}, ErrInvalidCredentials
		}
		return TokenPair{}, fmt.Errorf("select user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return TokenPair{}, ErrInvalidCredentials
	}

	return s.issueTokens(ctx, userID)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	tokenHash := hashToken(refreshToken)
	var tokenID, userID string
	var expiresAt time.Time
	var revokedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, expires_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&tokenID, &userID, &expiresAt, &revokedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TokenPair{}, ErrInvalidRefresh
		}
		return TokenPair{}, fmt.Errorf("select refresh token: %w", err)
	}
	if revokedAt.Valid || time.Now().UTC().After(expiresAt) {
		return TokenPair{}, ErrInvalidRefresh
	}

	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = $1 WHERE id = $2
	`, now, tokenID); err != nil {
		return TokenPair{}, fmt.Errorf("revoke refresh token: %w", err)
	}

	return s.issueTokens(ctx, userID)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := hashToken(refreshToken)
	_, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}

func (s *Service) Me(ctx context.Context, userID string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email FROM users WHERE id = $1
	`, userID).Scan(&user.ID, &user.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, fmt.Errorf("select user: %w", err)
	}
	return user, nil
}

func (s *Service) ParseAccessToken(token string) (string, error) {
	claims, err := s.tokenManager.Parse(token)
	if err != nil {
		return "", err
	}
	return claims.Sub, nil
}

func (s *Service) issueTokens(ctx context.Context, userID string) (TokenPair, error) {
	accessToken, err := s.tokenManager.Create(userID)
	if err != nil {
		return TokenPair{}, fmt.Errorf("create access token: %w", err)
	}

	refreshToken, err := randomToken()
	if err != nil {
		return TokenPair{}, fmt.Errorf("create refresh token: %w", err)
	}

	tokenID := newUUID()
	expiresAt := time.Now().UTC().Add(s.refreshTTL)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens(id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, tokenID, userID, hashToken(refreshToken), expiresAt)
	if err != nil {
		return TokenPair{}, fmt.Errorf("insert refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTTL.Seconds()),
	}, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newUUID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
