package grpcclient

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    "Thoth/proto/chatpb"
)

var clientLogger = slog.With("component", "grpc-client")

// ChatClient оборачивает gRPC клиент для Chat Service
type ChatClient struct {
    conn   *grpc.ClientConn
    client chatpb.ChatServiceClient
}

// NewChatClient создает новое подключение к Chat Service
func NewChatClient(address string) (*ChatClient, error) {
    clientLogger.Info("Connecting to Chat Service", "address", address)

    // Устанавливаем соединение с Chat Service
    // В продакшене здесь будут TLS credentials
    conn, err := grpc.Dial(address, 
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithTimeout(5*time.Second),
    )
    if err != nil {
        clientLogger.Error("Failed to connect to Chat Service", "error", err, "address", address)
        return nil, fmt.Errorf("failed to connect to chat service: %w", err)
    }

    client := chatpb.NewChatServiceClient(conn)
    
    clientLogger.Info("Successfully connected to Chat Service", "address", address)

    return &ChatClient{
        conn:   conn,
        client: client,
    }, nil
}

// SendMessage отправляет сообщение через gRPC
func (c *ChatClient) SendMessage(ctx context.Context, username, content, roomID string) (*chatpb.SendMessageResponse, error) {
    clientLogger.Info("Sending message via gRPC", 
        "username", username,
        "room_id", roomID,
        "content_length", len(content))

    // Создаем запрос
    req := &chatpb.ChatMessage{
        Username: username,
        Content:  content,
        RoomId:   roomID,
    }

    // Устанавливаем таймаут для запроса
    ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()

    // Отправляем запрос
    start := time.Now()
    resp, err := c.client.SendMessage(ctx, req)
    duration := time.Since(start)

    if err != nil {
        clientLogger.Error("gRPC SendMessage failed", 
            "error", err,
            "duration", duration,
            "username", username,
            "room_id", roomID)
        return nil, fmt.Errorf("grpc send message failed: %w", err)
    }

    clientLogger.Info("gRPC SendMessage successful", 
        "duration", duration,
        "success", resp.Success,
        "message_id", resp.MessageId)

    return resp, nil
}

// Close закрывает соединение с Chat Service
func (c *ChatClient) Close() error {
    if c.conn != nil {
        clientLogger.Info("Closing connection to Chat Service")
        return c.conn.Close()
    }
    return nil
}

// Health проверяет доступность Chat Service
func (c *ChatClient) Health(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()

    // Отправляем тестовое сообщение
    _, err := c.client.SendMessage(ctx, &chatpb.ChatMessage{
        Username: "health_check",
        Content:  "ping",
        RoomId:   "health",
    })

    if err != nil {
        clientLogger.Error("Chat Service health check failed", "error", err)
        return err
    }

    clientLogger.Info("Chat Service health check passed")
    return nil
}