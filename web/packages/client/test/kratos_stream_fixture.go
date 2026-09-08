package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	examplepb "github.com/Servora-Kit/plateau/api/gen/go/example/service/v1"
	httptransport "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/protobuf/types/known/structpb"
)

var statusRetryAttempts atomic.Int64

func main() {
	address := flag.String("address", ":18080", "监听地址")
	clientScript := flag.String("client-script", "", "浏览器客户端模块路径")
	flag.Parse()

	server := httptransport.NewServer(httptransport.Address(*address))
	route := server.Route("/")
	if *clientScript != "" {
		route.GET("/client.js", func(ctx httptransport.Context) error {
			http.ServeFile(ctx.Response(), ctx.Request(), *clientScript)
			return nil
		})
	}
	route.GET("/stream/ws-error", websocketErrorFixture)
	route.GET("/", fixtureIndex)
	route.GET("/stream/sse", sseFixture)
	route.GET("/stream/ws", websocketFixture)
	route.GET("/stream/ws-slow", func(ctx httptransport.Context) error {
		select {
		case <-time.After(250 * time.Millisecond):
			return websocketFixture(ctx)
		case <-ctx.Request().Context().Done():
			return ctx.Request().Context().Err()
		}
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := server.Stop(stopCtx); err != nil {
			log.Printf("停止流夹具失败：%v", err)
		}
	}()

	log.Printf("Kratos 流夹具监听 %s", *address)
	if err := server.Start(ctx); err != nil {
		log.Fatal(err)
	}
}

func fixtureIndex(ctx httptransport.Context) error {
	ctx.Response().Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, err := io.WriteString(ctx.Response(), "Kratos 流夹具已就绪\n")
	return err
}

func sseFixture(ctx httptransport.Context) error {
	switch ctx.Request().URL.Query().Get("case") {
	case "error":
		stream := httptransport.NewServerSentEventServerStream(ctx)
		if err := stream.Send(mustStruct(map[string]any{"sequence": 1.0})); err != nil {
			return err
		}
		return stream.Close(errors.New("夹具远端错误"))
	case "http-error":
		ctx.Response().Header().Set("Content-Type", "application/json")
		ctx.Response().WriteHeader(http.StatusBadRequest)
		_, err := io.WriteString(ctx.Response(), `{"code":400,"reason":"FIXTURE_INVALID","message":"夹具请求无效"}`)
		return err
	case "status-retry":
		if statusRetryAttempts.Add(1) == 1 {
			ctx.Response().Header().Set("Content-Type", "application/json")
			ctx.Response().WriteHeader(http.StatusServiceUnavailable)
			_, err := io.WriteString(ctx.Response(), `{"code":503,"reason":"FIXTURE_UNAVAILABLE","message":"夹具暂时不可用"}`)
			return err
		}
		stream := httptransport.NewServerSentEventServerStream(ctx)
		if err := stream.Send(mustStruct(map[string]any{"sequence": 1.0})); err != nil {
			return err
		}
		return stream.Close(nil)
	case "invalid-content-type":
		ctx.Response().Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(ctx.Response(), `{"sequence":1}`)
		return err
	case "invalid-json":
		ctx.Response().Header().Set("Content-Type", "text/event-stream")
		_, err := io.WriteString(ctx.Response(), "event: message\ndata: {\n\n")
		return err
	case "retry":
		return retrySSE(ctx)
	default:
		stream := httptransport.NewServerSentEventServerStream(ctx)
		if err := stream.Send(mustStruct(map[string]any{"sequence": 1.0})); err != nil {
			return err
		}
		time.Sleep(11 * time.Second)
		if err := stream.Send(mustStruct(map[string]any{"sequence": 2.0})); err != nil {
			return err
		}
		return stream.Close(nil)
	}
}

func retrySSE(ctx httptransport.Context) error {
	response := ctx.Response()
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	lastID := ctx.Request().Header.Get("Last-Event-ID")
	if lastID == "" {
		response.Header().Set("Content-Length", "4096")
		if _, err := io.WriteString(response, "id: fixture-1\nretry: 25\nevent: message\ndata: {\"sequence\":1}\n\n"); err != nil {
			return err
		}
		if flusher, ok := response.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(250 * time.Millisecond)
		return nil
	}
	if lastID != "fixture-1" {
		response.WriteHeader(http.StatusBadRequest)
		_, err := fmt.Fprintf(response, "Last-Event-ID 无效：%s", lastID)
		return err
	}
	_, err := io.WriteString(response, "id: fixture-2\nevent: message\ndata: {\"sequence\":2}\n\n")
	return err
}

func websocketFixture(ctx httptransport.Context) error {
	return runWebSocketFixture(ctx, false)
}

func websocketErrorFixture(ctx httptransport.Context) error {
	return runWebSocketFixture(ctx, true)
}

func runWebSocketFixture(ctx httptransport.Context, fail bool) error {
	stream, err := httptransport.NewWebSocketServerStream(ctx)
	if err != nil {
		return err
	}

	var inputs []map[string]any
	for {
		input := new(examplepb.DeleteUserRequest)
		err = stream.Recv(input)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return stream.Close(err)
		}
		inputs = append(inputs, map[string]any{
			"name":         input.GetName(),
			"etag":         input.GetEtag(),
			"allowMissing": input.GetAllowMissing(),
		})
		if fail {
			return stream.Close(errors.New("夹具远端错误"))
		}
	}

	var last any
	if len(inputs) > 0 {
		last = inputs[len(inputs)-1]
	}
	output := mustStruct(map[string]any{
		"count":         float64(len(inputs)),
		"last":          last,
		"query":         ctx.Request().URL.Query().Get("name"),
		"origin":        ctx.Request().Header.Get("Origin"),
		"cookiePresent": ctx.Request().Header.Get("Cookie") != "",
	})
	if err := stream.Send(output); err != nil {
		return stream.Close(err)
	}
	return stream.Close(nil)
}

func mustStruct(value map[string]any) *structpb.Struct {
	message, err := structpb.NewStruct(value)
	if err != nil {
		panic(err)
	}
	return message
}
