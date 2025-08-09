package chatservice

import (
    "context"
    "fmt"
    "log/slog"
    "time"
    
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    
    "Thoth/proto/chatpb"
    "Thoth/internal/storage"
)

var serviceLogger = slog.With("component", "chatservice")

// ChatService реализует gRPC интерфейс ChatServiceServer
type ChatService struct {
    chatpb.UnimplementedChatServiceServer
    store *storage.Storage
}

// NewChatService создает новый экземпляр Chat Service
func NewChatService(store *storage.Storage) *ChatService {
    serviceLogger.Info("Creating new ChatService instance")
    return &ChatService{
        store: store,
    }
}

// SendMessage обрабатывает отправку сообщения в чат
func (s *ChatService) SendMessage(ctx context.Context, req *chatpb.ChatMessage) (*chatpb.SendMessageResponse, error) {
    serviceLogger.Info("Received SendMessage request", 
        "username", req.Username,
        "room_id", req.RoomId,
        "content_length", len(req.Content))

    // Валидация входящих данных
    if req.Username == "" {
        serviceLogger.Warn("SendMessage: empty username")
        return &chatpb.SendMessageResponse{
            Success:      false,
            ErrorMessage: "Username cannot be empty",
        }, status.Error(codes.InvalidArgument, "username is required")
    }

    if req.Content == "" {
        serviceLogger.Warn("SendMessage: empty content")
        return &chatpb.SendMessageResponse{
            Success:      false,
            ErrorMessage: "Message content cannot be empty",
        }, status.Error(codes.InvalidArgument, "content is required")
    }

    if req.RoomId == "" {
        req.RoomId = "general" // Дефолтная комната
        serviceLogger.Info("SendMessage: using default room", "room_id", req.RoomId)
    }

    // Проверяем длину сообщения
    if len(req.Content) > 1000 {
        serviceLogger.Warn("SendMessage: message too long", "length", len(req.Content))
        return &chatpb.SendMessageResponse{
            Success:      false,
            ErrorMessage: "Message is too long (max 1000 characters)",
        }, status.Error(codes.InvalidArgument, "message too long")
    }

    // Создаем storage.Message для сохранения в БД
    storageMsg := storage.Message{
        Username: req.Username,
        Content:  req.Content,
        // storage.Message не имеет RoomID, но мы можем добавить позже
    }

    // Сохраняем в базу данных
    err := s.store.SaveMessage(ctx, storageMsg)
    if err != nil {
        serviceLogger.Error("Failed to save message to database", 
            "error", err,
            "username", req.Username,
            "room_id", req.RoomId)
        
        return &chatpb.SendMessageResponse{
            Success:      false,
            ErrorMessage: "Failed to save message",
        }, status.Error(codes.Internal, "database error")
    }

    // Генерируем уникальный ID сообщения (временно простой)
    messageID := fmt.Sprintf("%s_%d", req.RoomId, time.Now().UnixNano())

    serviceLogger.Info("Message saved successfully", 
        "message_id", messageID,
        "username", req.Username,
        "room_id", req.RoomId)

    // Возвращаем успешный ответ
    return &chatpb.SendMessageResponse{
        Success:      true,
        MessageId:    messageID,
        ErrorMessage: "",
    }, nil
}

// Дополнительные методы можно добавить позже:

// GetRecentMessages - получение истории сообщений
// func (s *ChatService) GetRecentMessages(ctx context.Context, req *chatpb.GetMessagesRequest) (*chatpb.GetMessagesResponse, error) {
//     // TODO: Реализовать позже
// }

// JoinRoom - присоединение к комнате  
// func (s *ChatService) JoinRoom(ctx context.Context, req *chatpb.JoinRoomRequest) (*chatpb.JoinRoomResponse, error) {
//     // TODO: Реализовать позже
// }