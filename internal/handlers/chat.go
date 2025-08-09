package handlers

import (
    "net/http"
    "log/slog"

    "github.com/gorilla/websocket"
    "Thoth/internal/models"
    "Thoth/internal/storage"
    wsHub "Thoth/internal/websocket"
)
var chatLogger = slog.With("component", "chat")

type ChatHandler struct {
    Hub *wsHub.Hub
    Store *storage.Storage
}

func NewChatHandler(hub *wsHub.Hub, store *storage.Storage) *ChatHandler {
    return &ChatHandler{Hub: hub, Store: store}
}

// ServeWS обрабатывает WebSocket подключения
func (ch *ChatHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
    // Получаем параметры из URL
    username := r.URL.Query().Get("username")
    roomID := r.URL.Query().Get("room")
    
    if username == "" {
        username = "Аноним"
    }
    if roomID == "" {
        roomID = "general"
    }
    
    chatLogger.Info("WebSocket connection attempt", 
        "username", username, 
        "room", roomID,
        "origin", r.Header.Get("Origin"),
        "remote", r.RemoteAddr)

    // Превращаем HTTP запрос в WebSocket соединение
    upgrader := &websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool {
            origin := r.Header.Get("Origin")
            // Для разработки разрешаем localhost
            allowedOrigins := []string{
                "https://localhost:8443",
                "https://thoth-webrtc.duckdns.org:8443",
                "https://127.0.0.1:8443",
            }
            
            for _, allowed := range allowedOrigins {
                if origin == allowed {
                    return true
                }
            }
            
            return true // Временно, для отладки
        },

        // БУФЕРЫ ДЛЯ WebRTC
        ReadBufferSize:  4096, 
        WriteBufferSize: 4096,  
    }

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        chatLogger.Error("Error WebSocket upgrade", "error", err)
        return
    }

    chatLogger.Info("WebSocket connection established for the client in the room", "username", username, "room", roomID)

    // Создаем нового клиента
    client := &wsHub.Client{
        Hub:      ch.Hub,
        Conn:     conn,
        Send:     make(chan models.Message, 1024),
        Username: username,
        RoomID:   roomID,
        Store:    ch.Store,
    }

    // Регистрируем клиента в Hub
    ch.Hub.Register <- client

    // Запускаем горутины для чтения и записи
    go client.WritePump()
    go client.ReadPump()
}