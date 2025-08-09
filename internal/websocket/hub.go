package websocket

import (
    "encoding/json"
    "log/slog"
    "time"
    "context"
    
    "github.com/gorilla/websocket"
    "Thoth/internal/models"
    "Thoth/internal/storage"
)

var hubLogger = slog.With("component", "hub")

// Client представляет одного подключенного пользователя
type Client struct {
    Hub      *Hub                   // Ссылка на главный Hub
    Conn     *websocket.Conn        // WebSocket соединение
    Send     chan models.Message    // Канал для отправки сообщений этому клиенту
    Username string                 // Имя пользователя
    RoomID   string                 // В какой комнате находится
    Store    *storage.Storage          // Отправка сообщений в БД
}

// Hub управляет всеми клиентами и сообщениями
type Hub struct {
    // Активные клиенты по комнатам
    Clients map[string]map[*Client]bool // [roomID][client] = active
    
    // Каналы для коммуникации
    Broadcast  chan models.Message  // Канал для рассылки сообщений
    Register   chan *Client         // Канал для регистрации новых клиентов  
    Unregister chan *Client         // Канал для отключения клиентов

    ctx    context.Context
    cancel context.CancelFunc
}

// NewHub создает новый Hub
func NewHub() *Hub {
    ctx, cancel := context.WithCancel(context.Background())
    
    return &Hub{
        Clients:    make(map[string]map[*Client]bool),
        Broadcast:  make(chan models.Message, 1000), // БУФЕР
        Register:   make(chan *Client),
        Unregister: make(chan *Client),
        ctx:        ctx,
        cancel:     cancel,
    }
}

func (h *Hub) Run() {
    hubLogger.Info("Hub is running and waiting for an event")
    for {
        select {
        case <-h.ctx.Done():
            hubLogger.Info("Received termination signal, exit Run()")
            h.shutdown()
            return

        case client := <-h.Register:
            hubLogger.Info("Registration request", "username", client.Username)
            
            if h.Clients[client.RoomID] == nil {
                h.Clients[client.RoomID] = make(map[*Client]bool)
                hubLogger.Info("Room created", "room", client.RoomID)
            }
            h.Clients[client.RoomID][client] = true
            
            clientCount := len(h.Clients[client.RoomID])
            hubLogger.Info("Client connected",
                "username", client.Username,
                "room", client.RoomID,
                "total_clients", clientCount,
            )

            // АСИНХРОННО уведомляем всех о новом пользователе
            joinMessage := models.Message{
                Type:      models.MessageTypeUserJoined,
                Username:  client.Username,
                Content:   client.Username + " присоединился к чату",
                Timestamp: time.Now(),
                RoomID:    client.RoomID,
            }
            hubLogger.Info("Send joinMessage asynchronously")
            h.SendMessageAsync(joinMessage)

            // АСИНХРОННО отправляем список пользователей
            hubLogger.Info("Sending a list of clients asynchronously")
            h.BroadcastUsersList(client.RoomID)

        case client := <-h.Unregister:
            hubLogger.Info("Received a request to disconnect the client", "username", client.Username)
            
            if clients, ok := h.Clients[client.RoomID]; ok {
                if _, ok := clients[client]; ok {
                    delete(h.Clients[client.RoomID], client)
                    close(client.Send)
                    hubLogger.Info("The client has disconnected from the room", "username", client.Username, "room", client.RoomID)

                    leaveMessage := models.Message{
                        Type:      models.MessageTypeUserLeft,
                        Username:  client.Username,
                        Content:   client.Username + " покинул чат",
                        Timestamp: time.Now(),
                        RoomID:    client.RoomID,
                    }
                    h.SendMessageAsync(leaveMessage)
                    h.BroadcastUsersList(client.RoomID)
                }
            }

        case message := <-h.Broadcast:
            hubLogger.Info("Received a message for distribution", 
                "type", message.Type,
                "username", message.Username,
                "room", message.RoomID)
                
            // WebRTC сообщение?
            if message.Type == models.MessageTypeWebRTCOffer || 
               message.Type == models.MessageTypeWebRTCAnswer || 
               message.Type == models.MessageTypeWebRTCCandidate {
                
                // WebRTC сообщения идут конкретному пользователю
                hubLogger.Info("WebRTC message for the client", "type", message.Type, "target", message.TargetUser)
                h.SendToUser(message)
            } else {
                // Обычные сообщения - всем в комнате
                if clients, ok := h.Clients[message.RoomID]; ok {
                        hubLogger.Info("Clients found in the room", "client_count", len(clients), "room", message.RoomID)
                        sentCount := 0
                        for client := range clients {
                            hubLogger.Info("Trying to send a message to the client", "username", client.Username)
                            select {
                            case client.Send <- message:
                                sentCount++
                                hubLogger.Info("The message has been successfully sent to the client", "username", client.Username)
                            default:
                                hubLogger.Error("The client's queue is full, disconnecting the client", "username", client.Username)
                                close(client.Send)
                                delete(h.Clients[message.RoomID], client)
                            }
                        }
                        hubLogger.Info("Message sent to clients", "sent_count", sentCount)
                } else {
                    hubLogger.Error("Room not found in h.Clients", "room", message.RoomID)
                }
            }
        }
    }
}

// ReadPump читает сообщения от браузера и отправляет в Hub
func (c *Client) ReadPump() {
    defer func() {
        hubLogger.With("method", "readpump").Info("Completion for the client", "username", c.Username)
        c.Hub.Unregister <- c  // При выходе - отключаемся от Hub
        c.Conn.Close()         // Закрываем WebSocket соединение
    }()

    // Настройки соединения
    c.Conn.SetReadLimit(32768)  // 32Kb
    c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    
    // Обработчик pong (для keep-alive)
    c.Conn.SetPongHandler(func(string) error {
        c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
        return nil
    })

    for {
        var msg models.Message
        
        // Читаем JSON сообщение от браузера
        err := c.Conn.ReadJSON(&msg)
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                hubLogger.With("method", "readpump").Error("WebSocket error for the client", "username", c.Username, "error", err)
            } else {
                hubLogger.With("method", "readpump").Error("Normal connection closure for the client", "username", c.Username, "error", err)
            }
            break
        }

        // Заполняем метаданные сообщения
        msg.Username = c.Username
        msg.RoomID = c.RoomID
        msg.Timestamp = time.Now()

        if msg.Type == models.MessageTypeChat && c.Store != nil {
            err := c.Store.SaveMessage(c.Hub.ctx, storage.Message{
                Username: msg.Username,
                Content:  msg.Content,
            })
            if err != nil {
                hubLogger.With("method", "readpump").Error("Error saving message to database", "error", err)
            }
        }

        // Если без типа - обычный чат
        if msg.Type == "" {
            msg.Type = models.MessageTypeChat
        }

        // ЛОГИРУЕМ WEBRTC СООБЩЕНИЯ ОТДЕЛЬНО
        if msg.Type == models.MessageTypeWebRTCOffer || 
           msg.Type == models.MessageTypeWebRTCAnswer || 
           msg.Type == models.MessageTypeWebRTCCandidate {
            hubLogger.With("method", "readpump").Info("Received WebRTC message from", 
                "username", c.Username,
                "type", msg.Type,
                "target", msg.TargetUser)
        } else {
            hubLogger.With("method", "readpump").Info("Recieved gRPC message from", 
                "username", c.Username,
                "type", msg.Type,
                "content", msg.Content)
        }

        // Отправляем в Hub для рассылки
        select {
        case c.Hub.Broadcast <- msg:
            // Сообщение отправлено в Hub
        default:
            hubLogger.With("method", "readpump").Error("Broadcast is full! Message from the client lost", "username", c.Username)
        }
    }
}

// WritePump отправляет сообщения из канала Send в браузер
func (c *Client) WritePump() {
    ticker := time.NewTicker(54 * time.Second)  // Ping каждые 54 секунды
    defer func() {
        ticker.Stop()
        c.Conn.Close()
        hubLogger.With("method", "writepump").Info("Completion for the client", "username", c.Username)
    }()

    hubLogger.With("method", "writepump").Info("WritePump is running for the client", "username", c.Username)

    for {
        select {
        case message, ok := <-c.Send:
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            
            if !ok {
                // Hub закрыл канал - отправляем close message
                hubLogger.With("method", "writepump").Error("Channel Send is closed for the client", "username", c.Username)
                c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            // ЛОГИРУЕМ WEBRTC ОТПРАВКУ
            if message.Type == models.MessageTypeWebRTCOffer || 
               message.Type == models.MessageTypeWebRTCAnswer || 
               message.Type == models.MessageTypeWebRTCCandidate {
                hubLogger.With("method", "writepump").Info("Sending WebRTC message for the client", 
                    "type", message.Type,
                    "username", c.Username)
            }

            // Отправляем JSON сообщение в браузер
            if err := c.Conn.WriteJSON(message); err != nil {
                hubLogger.With("method", "writepump").Error("Error sending message for the client", "username", c.Username, "error", err)
                return
            }

        case <-ticker.C:
            // Отправляем ping для поддержания соединения
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                hubLogger.With("method", "writepump").Error("Ping error for the client", "username", c.Username, "error", err)
                return
            }
        }
    }
}

func (h *Hub) FindClient(roomID, username string) *Client {
    if clients, exists := h.Clients[roomID]; exists {
        for client := range clients {
            if client.Username == username {
                return client
            }
        }
    }
    return nil
}

// Отправить сообщение конкретному пользователю
func (h *Hub) SendToUser(message models.Message) {
    targetClient := h.FindClient(message.RoomID, message.TargetUser)
    if targetClient == nil {
        hubLogger.With("method", "sendtouser").Error("The client was not found in the room", "target", message.TargetUser, "room", message.RoomID)
        return
    }
    
    select {
    case targetClient.Send <- message:
        hubLogger.With("method", "sendtouser").Info("Send WebRTC message to the cient", "type", message.Type, "target", message.TargetUser)
    default:
        hubLogger.With("method", "sendtouser").Error("The client queue is full, disconnect client", "target", message.TargetUser)
        close(targetClient.Send)
        delete(h.Clients[message.RoomID], targetClient)
    }
}

func (h *Hub) GetRoomUsers(roomID string) []string {
    var users []string
    if clients, ok := h.Clients[roomID]; ok {
        for client := range clients {
            users = append(users, client.Username)
        }
    }
    return users
}

// Отправляет список пользователей всем в комнате
func (h *Hub) BroadcastUsersList(roomID string) {
    users := h.GetRoomUsers(roomID)
    
    // Преобразуем список пользователей в JSON строку
    usersJSON, err := json.Marshal(users)
    if err != nil {
        hubLogger.With("method", "broadcastuserslist").Error("Failed to serialize users list", "room", roomID, "error", err)
        return
    }
    
    // Создаем сообщение с типом users_list
    usersMessage := models.Message{
        Type:      models.MessageTypeUsersList,
        Content:   string(usersJSON), // JSON строка со списком пользователей
        RoomID:    roomID,
        Timestamp: time.Now(),
        Username:  "system",
    }
    
    // АСИНХРОННАЯ отправка в отдельной горутине
    go func() {
        select {
        case h.Broadcast <- usersMessage:
            // Отправлено
        default:
            hubLogger.With("method", "broadcastuserslist").Error("Failed to send users_list - Broadcast if full")
        }
    }()
}

// Метод для асинхронной отправки сообщений
func (h *Hub) SendMessageAsync(message models.Message) {
    go func() {
        select {
        case h.Broadcast <- message:
            // Отправлено
        default:
            hubLogger.With("method", "sendmessageasync").Error("Failed to send async message - Broadcast is full")
        }
    }()
}

func (h *Hub) Stop() {
    hubLogger.With("method", "stop").Info("Shutting down via context")
    h.cancel()
}

func (h *Hub) shutdown() {
    hubLogger.With("method", "shutdown").Info("Completing the connections")
    for _, clients := range h.Clients {
        for client := range clients {
            close(client.Send)
            client.Conn.Close()
        }
    }
}