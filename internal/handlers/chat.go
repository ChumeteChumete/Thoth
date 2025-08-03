package handlers

import (
    "log"
    "net/http"

    "github.com/gorilla/websocket"
    "Thoth/internal/models"
    wsHub "Thoth/internal/websocket"
)

type ChatHandler struct {
    Hub *wsHub.Hub
}

func NewChatHandler(hub *wsHub.Hub) *ChatHandler {
    return &ChatHandler{Hub: hub}
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
    }

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("Ошибка WebSocket upgrade: %v", err)
        return
    }

    // Создаем нового клиента
    client := &wsHub.Client{
        Hub:      ch.Hub,
        Conn:     conn,
        Send:     make(chan models.Message, 256),
        Username: username,
        RoomID:   roomID,
    }

    // Регистрируем клиента в Hub
    ch.Hub.Register <- client

    // Запускаем горутины для чтения и записи
    go client.WritePump()
    go client.ReadPump()
}