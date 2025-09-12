package auth

import (
	"fmt"
	"errors"
	"time"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "auth-service/proto"
)

const (
	queryInsertUser = `insert into users (email, passhash) values ($1, $2)`
	queryGetUser = `select user_id, passhash from users where login = $1`
)

const (
    accessTTL  = 15 * time.Minute
    refreshTTL = 24 * time.Hour
)

var (
    ErrInvalidToken   = errors.New("недействительный токен")
    ErrTokenTampered  = errors.New("подмена токена")
    ErrInvalidCreds   = errors.New("неверный логин или пароль")
)


type server struct {
	pb.UnimplementedAuthServiceServer
	db *pgxpool.Pool
	key *ecdsa.PrivateKey
}

func (s *server) parseAndValidateToken(tokenStr string, expectedID string) (*jwt.RegisteredClaims, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("Ошибка подписи ключа: %v", token.Header["alg"])
		}
		return &s.key.PublicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("Ошибка токена: %v", err)
	}
	
	if !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, jwt.ErrInvalidKey)
	}
	if claims.ID != expectedID {
		return nil, fmt.Errorf("%w: %v", ErrTokenTampered, jwt.ErrTokenInvalidId)
	}
	return claims, nil
}

func (s *server) generateToken(userID, jti string, ttl time.Duration) (string, error) {
	token := jwt.NewWithClaims(
		jwt.SigningMethodES256,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			Subject: userID,
			ID: jti
		}
	)
	return token.SignedString(s.key)
}
// TODO:
// func (s *server) blacklistToken(ctx context.Context, claims *jwt.RegisteredClaims) error { ... }

// func (s *server) isBlacklisted(ctx context.Context, claims *jwt.RegisteredClaims) (bool, error) { ... }

func (s *server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.Emptymessage, error) {
	// noqa колонку email в таблице users unique обязательно!

	passhash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("Ошибка хеширования пароля %v", err)
	}

	_, err = s.db.Exec(ctx, queryInsertUser, req.Login, string(passhash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("Пользователь с логином %s уже существует", req.Login)
		}
		return nil, fmt.Errorf("Не удалось зарегистрировать пользователя: %v", err)
	}
	return &pb.Emptymessage{}, nil
}

func (s *server) SignIn(ctx context.Context, req *pb.SignInRequest) (*pb.SignInResponse, error) {
	// Аутентификация
	var userID uuid.UUID
	var passhash string

	err := s.db.QueryRow(ctx, queryGetUser, req.Login).Scan(&userID, &passhash)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w", ErrInvalidCreds)
		}
		return nil, fmt.Errorf("Ошибка бд: %v", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(passhash), []byte(req.Password))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCreds, err)
	}

	accessToken, err := s.generateToken(userID.String(), "access", accessTTL)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.generateToken(userID.String(), "refresh", refreshTTL)
	if err != nil {
		return nil, err
	}

	return &pb.SignInResponse{AccessJWT: accessToken, RefreshJWT: refreshToken}
}

func (s *server) Refresh(ctx context.Context, req *pb.RefreshJWT) (*pb.AccessJWT, error) {
	claims, err := s.parseAndValidateToken(req.RefreshJWT, "refresh")
	if err != nil {
		return nil, err
	}

	userID, err := claims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("Ошибка поля пользователя: %v", err)
	}

	accessToken, err := s.generateToken(userID, "access", accessTTL)

	return &pb.AccessJWT{AccessJWT: accessToken}
}

func (s *server) AuthCheck(ctx context.Context, req *pb.AccessJWT) (*pb.Emptymessage, error) {
	claims, err := s.parseAndValidateToken(req.AccessJWT, "access")
	if err != nil {
		return nil, err
	}

	return &pb.Emptymessage{}, nil
}

func (s *server) SignOut(ctx context.Context, req *pb.RefreshJWT) (*pb.Emptymessage, error) {
	claims, err := s.parseAndValidateToken(req.RefreshJWT, "refresh")
	if err != nil {
		return nil, err
	}
	// логика добавления refresh и/или access в redis.BlackList
	return &pb.Emptymessage{}, nil
}

func main() {
	conn, err := pgxpool.New(context.Background(), "DATABASE_URL")
	if err != nil {
		fmt.Errorf("Не удалось подключиться к базе данных %v", err)
	}
	defer conn.Close(context.Background())

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fmt.Errorf("Ошибка генерации ключа подписи: %v", err)
		return
	}

	s := &server{db: conn, key: k}
	// TODO: finalize main()
}