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

type server struct {
	pb.UnimplementedAuthServiceServer
	db *pgxpool.Pool
	key *ecdsa.PrivateKey
}

func (s *server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.Emptymessage, error) {
	// noqa колонку email в таблице users unique обязательно!

	passhash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("Ошибка хеширования пароля %v", err)
	}

	_, err = s.db.Exec(ctx, "insert into users (email, passhash) values ($1, $2)", req.Login, string(passhash))
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
	var userId uuid.UUID
	var passhash string

	err := s.db.QueryRow(
		ctx, 
		"select user_id, passhash
		from users 
		where login = $1", 
		req.Login).Scan(&userId, &passhash)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("Неверный логин или пароль")
		}
		return nil, fmt.Errorf("Ошибка бд: %v", err)
	}

	err = CompareHashAndPassword([]byte(passhash), []byte(req.Password))
	if err != nil {
		return nil, fmt.Errorf("Неверный пароль: %v", err)
	}

	var (
		key *ecdsa.PrivateKey
		accessT, refreshT *jwt.Token
		accessToken, refreshToken string
	)
	
	accessT = jwt.NewWithClaims(
		jwt.SigningMethodES256,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			Subject: userId.String(),
			ID: "access"
		}
	)
	accessToken, err = accessT.SignedString(s.key)
	if err != nil {
		return nil, fmt.Errorf("Ошибка подписания access: %v", err)
	}

	refreshT = jwt.NewWithClaims(
		jwt.SigningMethodES256,
		jwt.RegisteredClaims(
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			Subject: userId.String(),
			ID: "refresh"
		)
	)
	refreshToken, err = refreshT.SignedString(s.key)
	if err != nil {
		return nil, fmt.Errorf("Ошибка подписания refresh: %v", err)
	}

	return &pb.SignInResponse{AccessJWT: accessToken, RefreshJWT: refreshToken}
}

func (s *server) Refresh(ctx context.Context, req *pb.RefreshJWT) (*pb.AccessJWT, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := ParseWithClaims(req.RefreshJWT, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method(*jwt.SigningMethodECDSA); !ok {
			return nil, Errorf("Ошибка подписи ключа: %v", token.Header["alg"])
		}
		return &s.key.PublicKey, nil
	})
	
	switch {
	case token.Valid:
		if ID := token.Claims.ID; ID != "refresh" {
			return nil, fmt.Errorf("Подмена токена: %v", jwt.ErrTokenInvalidId)
		}
		userId, err := token.Claims.GetSubject()
		if err != nil {
			return nil, Errorf("Ошибка поля пользователя: %v", err)
		}
		var (
			accessT *jwt.Token
			accessToken string
		)
		accessT = jwt.NewWithClaims(
			jwt.SigningMethodES256,
			jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
				Subject: userId.String()
			}
		)
		accessToken, err = accessT.SignedString(s.key)
		if err != nil {
			return nil, fmt.Errorf("Ошибка подписания access после refresh: %v", err)
		}
		return &pb.AccessJWT{AccessJWT: accessToken}
	case errors.Is(err, jwt.ErrTokenMalformed):
		return nil, fmt.Errorf("Токен искажен: %v", err)
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return nil, fmt.Errorf("Ошибка подписи токена: %v", err)
	case errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenNotValidYet):
		return nil, fmt.Errorf("Токен просрочен: %v", err)
	default:
		return nil, fmt.Errorf("Ошибка обработки токена: %v", err)
	}
}

func main() {
	conn, err := pgxpool.New(context.Background(), "DATABASE_URL")
	if err != nil {
		fmt.Errorf("Не удалось подключиться к базе данных %v", err)
	}
	defer conn.Close(context.Background())

	var k *ecdsa.PrivateKey
	k, err = ecdsa.GenerateKey(elliptic.P256, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Ошибка генерации ключа подписи: %v", err)
	}

	s := &server{db: conn, key: k}
	// TODO: finalize main()
}