package grpc

import (
	"capsule-me/internal/domain/catalog"
	capsulegen "capsule-me/internal/gen/capsule-gen"
	"capsule-me/internal/gen/common"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// это то, откуда воркеры берут работу, id надо для индентификации запроса, инкремент
type job struct {
	id     int32
	req    *catalog.IncomingFeature
	ctx    context.Context
	result chan result
}

type result struct {
	id   int32
	resp *catalog.Capsule
	err  error
}

type GrpcCatalogClient struct {
	mu        *sync.RWMutex
	wg        *sync.WaitGroup
	stopped   atomic.Bool
	workers   int
	increment func() int32
	client    capsulegen.CapsuleGenServiceClient
	jobs      chan job
	log       *slog.Logger
}

func inc() func() int32 {
	var i atomic.Int32
	return func() int32 {
		i.Add(1)
		return i.Load()
	}
}

func NewGrpcCatalogClient(maxWorkers int, baseURL string, logger *slog.Logger) (*GrpcCatalogClient, error) {
	conn, err := grpc.NewClient(baseURL, grpc.WithTransportCredentials(insecure.NewCredentials())) // DialOptions сделать потом, они идут как второй аргумент ...DialOptions
	if err != nil {
		return nil, err
	}
	jobs := make(chan job, maxWorkers)

	client := capsulegen.NewCapsuleGenServiceClient(conn)
	if client == nil {
		return nil, errors.New("nil client in NewGrpcCatalogClient")
	}
	wg := &sync.WaitGroup{}
	mu := &sync.RWMutex{}
	increment := inc()

	return &GrpcCatalogClient{
		mu:        mu,
		wg:        wg,
		client:    client,
		jobs:      jobs,
		workers:   maxWorkers,
		increment: increment,
		log:       logger,
	}, nil
}

// заводит воркеров
func (cc *GrpcCatalogClient) Start() {
	for i := 0; i < cc.workers; i++ {
		cc.wg.Add(1)
		go func() {
			defer cc.wg.Done()
			for j := range cc.jobs {
				res, err := cc.Srecommend(j.ctx, j.req)
				j.result <- result{id: j.id, resp: res, err: err}
			}
		}()
	}
	cc.log.Info("grpc pool started")
}

func (cc *GrpcCatalogClient) Stop() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.stopped.Store(true)
	close(cc.jobs)
	cc.log.Info("каналы закрыты")

	done := make(chan struct{})
	go func() {
		cc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		cc.log.Info("воркеры остановлены")
	case <-time.After(5 * time.Second):
		cc.log.Error("таймаут ожидания воркеров")
	}
}

func toDTO(in *catalog.IncomingFeature) (*capsulegen.CapsuleGenerateRequest, error) {
	if in != nil {
		res := &capsulegen.CapsuleGenerateRequest{}

		switch in.Gender {
		case "male":
			res.Gender = common.Gender_GENDER_MALE
		case "female":
			res.Gender = common.Gender_GENDER_FEMALE
		default:
			res.Gender = common.Gender_GENDER_UNSPECIFIED
		}

		switch in.Style {
		case "classic":
			res.Style = common.Style_STYLE_CLASSIC
		case "casual":
			res.Style = common.Style_STYLE_CASUAL
		case "sport":
			res.Style = common.Style_STYLE_SPORT
		default:
			res.Style = common.Style_STYLE_UNSPECIFIED
		}

		switch in.Season {
		case "winter":
			res.Season = common.Season_SEASON_WINTER
		case "summer":
			res.Season = common.Season_SEASON_SUMMER
		case "autumn":
			res.Season = common.Season_SEASON_AUTUMN
		case "spring":
			res.Season = common.Season_SEASON_SPRING
		default:
			res.Season = common.Season_SEASON_UNSPECIFIED
		}

		switch in.Color {
		case "dark":
			res.Palette = common.Palette_PALETTE_DARK.Enum()
		case "light":
			res.Palette = common.Palette_PALETTE_LIGHT.Enum()
		case "bright":
			res.Palette = common.Palette_PALETTE_BRIGHT.Enum()
		case "none":
			res.Palette = nil
		default:
			res.Palette = common.Palette_PALETTE_UNSPECIFIED.Enum()
		}

		return res, nil
	}

	return nil, errors.New("grpc toDTO: in is nil")
}

// картинка перекладывается отдельно
func fromDTO(out *capsulegen.Capsule) (*catalog.Capsule, error) {
	if out != nil {
		res := &catalog.Capsule{}

		for _, item := range out.Item {
			i := &catalog.ImageItem{}
			id, err := uuid.Parse(item.GetId())
			if err != nil {
				return nil, errors.New("invalid item ID")
			}
			i.ID = id
			i.ObjectID = id
			i.Gender = item.GetGender().String()
			i.CategoryGroup = item.GetCategoryGroup()
			i.Category = item.GetCategory()
			i.Style = item.GetStyle().String()
			i.Color = item.GetColor()
			i.Season = item.GetSeason().String()
			i.Material = item.GetMaterial()
			i.Description = item.GetDescription()
			i.Ext = item.GetExt()

			res.Items = append(res.Items, *i)
		}

		return res, nil
	}
	return nil, errors.New("grpc fromDTO: out is nil")
}

func (cc *GrpcCatalogClient) submit(job job) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.stopped.Load() {
		return false
	}
	select {
	case cc.jobs <- job:
		return true
	default:
		return false
	}
}

func (cc *GrpcCatalogClient) Srecommend(ctx context.Context, f *catalog.IncomingFeature) (*catalog.Capsule, error) {
	dto, err := toDTO(f)
	if err != nil {
		cc.log.Error("to dto err", "err", err)
		return nil, err
	}

	stream, err := cc.client.CapsuleGenerate(ctx, dto)
	if err != nil {
		cc.log.Error("stream err", "err", err)
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
			cc.log.Error("stream Recv err", "err", err)
			return nil, err
		}

		switch v := resp.Payload.(type) {
		case *capsulegen.CapsuleGenerateResponse_Capsule:
			capsule = v.Capsule

		case *capsulegen.CapsuleGenerateResponse_ImageChunk:
			imageData = append(imageData, v.ImageChunk...)

		default:
			cc.log.Error("unknown payload")
		}
	}

	if capsule == nil || len(imageData) == 0 {
		cc.log.Error("nil капсула или картинка")
		return nil, errors.New("nil капсула или картинка")
	}

	res, err := fromDTO(capsule)
	if err != nil {
		cc.log.Error("from dto err")
		return nil, err
	}
	res.Image = imageData
	return res, nil
}

func (cc *GrpcCatalogClient) Recommend(ctx context.Context, f *catalog.IncomingFeature) (*catalog.Capsule, error) {
	resChan := make(chan result, cc.workers)

	job := job{
		id:     cc.increment(),
		req:    f,
		ctx:    ctx,
		result: resChan,
	}

	if ok := cc.submit(job); !ok {
		return nil, errors.New("job не положилась в канал")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	// case <-time.After(5 * time.Second):
	// return nil, errors.New("capsule timeout")
	case res := <-job.result:
		return res.resp, res.err
	}
}
