package grpc

import (
	"capsule-me/internal/domain/catalog"
	capsulegen "capsule-me/internal/gen/capsule-gen"
	"context"
	"errors"
	"io"

	"google.golang.org/grpc"
)

// это то, откуда воркеры берут работу, id надо для индентификации запроса, инкремент
type job struct {
	id  int
	req *catalog.IncomingFeature
	ctx context.Context
}

type result struct {
	id   int
	resp *catalog.Capsule
	err  error
}

type grpcCatalogClient struct {
	workers int
	client  capsulegen.CapsuleGenServiceClient
	jobs    chan job
	results chan result
}

// доделать
func NewGrpcCatalogClient(maxWorkers int, baseURL string, conn *grpc.ClientConn) *grpcCatalogClient {
	conn, err := grpc.NewClient(baseURL) // DialOptions сделать потом, они идут как второй аргумент ...DialOptions
	if err != nil {
		return nil
	}
	jobs := make(chan job, maxWorkers)
	results := make(chan result, maxWorkers)

	client := capsulegen.NewCapsuleGenServiceClient(conn)
	if client == nil {
		return nil
	}

	return &grpcCatalogClient{
		client:  client,
		jobs:    jobs,
		results: results,
		workers: maxWorkers,
	}
}

// заводит воркеров
func (cc *grpcCatalogClient) Start() {
	for i := 0; i <= cc.workers; i++ {
		go func() {
			for j := range cc.jobs {
				select {
				case <-j.ctx.Done():
					return
				default:
					res, err := cc.recommend(j.ctx, j.req)
					if err != nil {
						cc.results <- result{id: j.id, err: err}
					}
					cc.results <- result{id: j.id, resp: res, err: err}
				}
			}
		}()
	}
}

// реализовать
func toDTO(in *catalog.IncomingFeature) (*capsulegen.CapsuleGenerateRequest, error)
func fromDTO(out *capsulegen.Capsule) (*catalog.Capsule, error)

// данный метод будет просто синхронным, он будет использоваться просто для работы воркера
func (cc *grpcCatalogClient) recommend(ctx context.Context, f *catalog.IncomingFeature) (*catalog.Capsule, error) {
	dto, err := toDTO(f)
	if err != nil {
		return nil, err
	}

	stream, err := cc.client.CapsuleGenerate(ctx, dto)
	if err != nil {
		return nil, err
	}

	var capsule *capsulegen.Capsule
	var imageData []byte

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch v := resp.Payload.(type) {
		case *capsulegen.CapsuleGenerateResponse_Capsule:
			capsule = v.Capsule
			// тут можно логгировать

		case *capsulegen.CapsuleGenerateResponse_ImageChunk:
			imageData = append(imageData, v.ImageChunk...)
			// тут тоже логгировать

		default:
			// логгировать неизвестный тип payload
		}
	}

	if capsule == nil || len(imageData) == 0 {
		return nil, errors.New("nil капсула или картинка")
	}

	res, err := fromDTO(capsule)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// в самом методе основном, который будет исполнять интерфейс, он просто достает из канала результатов нужный результат
