package main

import (
	"fmt"
    "context"
    "log/slog"
    "net"
    "os"
    "os/signal"
    "syscall"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
    "github.com/joho/godotenv"

    "Thoth/internal/chatservice"
    "Thoth/internal/storage"
    "Thoth/proto/chatpb"
)

var serverLogger = slog.With("component", "grpc-server")

func main() {
    // Загружаем .env файл
    if err := godotenv.Load("../../.env"); err != nil {
        serverLogger.Warn("File .env not found, using system environment variables")
    }

    serverLogger.Info("Starting Chat Service gRPC Server")

    // Подключаемся к базе данных
    connStr := os.Getenv("THOTH_DB_CONN")
    if connStr == "" {
        serverLogger.Error("Environment variable THOTH_DB_CONN is not set")
        os.Exit(1)
    }

    store, err := storage.NewStorage(connStr)
    if err != nil {
        serverLogger.Error("Failed to connect to database", "error", err)
        os.Exit(1)
    }
    defer store.Close()
    serverLogger.Info("Database connection established")

    // Создаем Chat Service
    chatSvc := chatservice.NewChatService(store)

    // Создаем gRPC сервер
    grpcServer := grpc.NewServer(
        grpc.UnaryInterceptor(loggingInterceptor),
    )

    // Регистрируем наш сервис
    chatpb.RegisterChatServiceServer(grpcServer, chatSvc)

    // Включаем reflection для отладки (можно отключить в продакшене)
    reflection.Register(grpcServer)

    // Слушаем порт 9090
    lis, err := net.Listen("tcp", ":9090")
    if err != nil {
        serverLogger.Error("Failed to listen on port 9090", "error", err)
        os.Exit(1)
    }

    serverLogger.Info("gRPC server is listening", "address", lis.Addr())

    // Запускаем сервер в отдельной горутине
    go func() {
        if err := grpcServer.Serve(lis); err != nil {
            serverLogger.Error("gRPC server failed", "error", err)
            os.Exit(1)
        }
    }()

    serverLogger.Info("🚀 Chat Service gRPC Server is running on :9090")

    // Graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    <-ctx.Done()
    serverLogger.Info("Shutdown signal received")

    // Даем серверу 5 секунд на завершение
    shutdownTimer := time.NewTimer(5 * time.Second)
    defer shutdownTimer.Stop()

    done := make(chan struct{})
    go func() {
        grpcServer.GracefulStop()
        close(done)
    }()

    select {
    case <-done:
        serverLogger.Info("gRPC server stopped gracefully")
    case <-shutdownTimer.C:
        serverLogger.Warn("Force stopping gRPC server")
        grpcServer.Stop()
    }

    serverLogger.Info("Chat Service shutdown complete")
}

// loggingInterceptor логирует все gRPC запросы
func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    start := time.Now()
    
    serverLogger.Info("gRPC request started", 
        "method", info.FullMethod,
        "request", fmt.Sprintf("%+v", req))

    // Выполняем запрос
    resp, err := handler(ctx, req)
    
    duration := time.Since(start)
    
    if err != nil {
        serverLogger.Error("gRPC request failed", 
            "method", info.FullMethod,
            "duration", duration,
            "error", err)
    } else {
        serverLogger.Info("gRPC request completed", 
            "method", info.FullMethod,
            "duration", duration,
            "response", fmt.Sprintf("%+v", resp))
    }

    return resp, err
}