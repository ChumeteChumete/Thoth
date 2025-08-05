package handlers

import (
    "log"
    "net/http"

    "github.com/gorilla/websocket"
    "Thoth/internal/models"
    "Thoth/internal/storage"
    wsHub "Thoth/internal/websocket"
)

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

    // Превращаем HTTP запрос в WebSocket соединение
    upgrader := &websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool {
            return true
        },
        // БУФЕРЫ ДЛЯ WebRTC
        ReadBufferSize:  4096, 
        WriteBufferSize: 4096,  
    }

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("Ошибка WebSocket upgrade: %v", err)
        return
    }

    log.Printf("WebSocket подключение установлено для %s в комнате %s", username, roomID)

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