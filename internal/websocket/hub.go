package websocket

import (
    "encoding/json"
    "log"
    "time"
    "context"
    
    "github.com/gorilla/websocket"
    "Thoth/internal/models"
    "Thoth/internal/storage"
)

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
    log.Println("Hub запущен и ожидает события")
    for {
        select {
        case <-h.ctx.Done():
            log.Println("Hub: Получен сигнал завершения, выходим из Run()")
            h.shutdown()
            return

        case client := <-h.Register:
            log.Printf("Hub: Получен запрос на регистрацию клиента %s", client.Username)
            
            if h.Clients[client.RoomID] == nil {
                h.Clients[client.RoomID] = make(map[*Client]bool)
                log.Printf("Hub: Создана новая комната %s", client.RoomID)
            }
            h.Clients[client.RoomID][client] = true
            
            clientCount := len(h.Clients[client.RoomID])
            log.Printf("Клиент %s подключился к комнате %s. Всего клиентов в комнате: %d", 
                client.Username, client.RoomID, clientCount)

            // АСИНХРОННО уведомляем всех о новом пользователе
            joinMessage := models.Message{
                Type:      models.MessageTypeUserJoined,
                Username:  client.Username,
                Content:   client.Username + " присоединился к чату",
                Timestamp: time.Now(),
                RoomID:    client.RoomID,
            }
            log.Printf("Hub: Отправляем joinMessage асинхронно")
            h.SendMessageAsync(joinMessage)

            // АСИНХРОННО отправляем список пользователей
            log.Printf("Hub: Отправляем список пользователей асинхронно")
            h.BroadcastUsersList(client.RoomID)

        case client := <-h.Unregister:
            log.Printf("Hub: Получен запрос на отключение клиента %s", client.Username)
            
            if clients, ok := h.Clients[client.RoomID]; ok {
                if _, ok := clients[client]; ok {
                    delete(h.Clients[client.RoomID], client)
                    close(client.Send)
                    log.Printf("Клиент %s отключился от комнаты %s", client.Username, client.RoomID)

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
            log.Printf("Hub: Получено сообщение для рассылки: тип='%s', от='%s', комната='%s'", 
                message.Type, message.Username, message.RoomID)
                
<<<<<<< HEAD
            if clients, ok := h.Clients[message.RoomID]; ok {
                log.Printf("Hub: Найдено %d клиентов в комнате %s", len(clients), message.RoomID)
                sentCount := 0
                for client := range clients {
                    log.Printf("Hub: Пытаемся отправить сообщение клиенту %s", client.Username)
                    select {
                    case client.Send <- message:
                        sentCount++
                        log.Printf("Hub: Сообщение успешно отправлено клиенту %s", client.Username)
                    default:
                        log.Printf("Hub: Очередь клиента %s переполнена, отключаем", client.Username)
                        close(client.Send)
                        delete(h.Clients[message.RoomID], client)
                    }
=======
            // WebRTC сообщение?
            if message.Type == models.MessageTypeWebRTCOffer || 
               message.Type == models.MessageTypeWebRTCAnswer || 
               message.Type == models.MessageTypeWebRTCCandidate {
                
                // WebRTC сообщения идут конкретному пользователю
                log.Printf("Hub: WebRTC сообщение '%s' для пользователя %s", message.Type, message.TargetUser)
                h.SendToUser(message)
            } else {
                // Обычные сообщения - всем в комнате
                if clients, ok := h.Clients[message.RoomID]; ok {
                        log.Printf("Hub: Найдено %d клиентов в комнате %s", len(clients), message.RoomID)
                        sentCount := 0
                        for client := range clients {
                            log.Printf("Hub: Пытаемся отправить сообщение клиенту %s", client.Username)
                            select {
                            case client.Send <- message:
                                sentCount++
                                log.Printf("Hub: Сообщение успешно отправлено клиенту %s", client.Username)
                            default:
                                log.Printf("Hub: Очередь клиента %s переполнена, отключаем", client.Username)
                                close(client.Send)
                                delete(h.Clients[message.RoomID], client)
                            }
                        }
                        log.Printf("Hub: Сообщение отправлено %d клиентам", sentCount)
                } else {
                    log.Printf("Hub: ОШИБКА - Комната %s не найдена в h.Clients!", message.RoomID)
>>>>>>> b7106ae (added postgreSQL, updated UI)
                }
                log.Printf("Hub: Сообщение отправлено %d клиентам", sentCount)
            } else {
                log.Printf("Hub: ОШИБКА - Комната %s не найдена в h.Clients!", message.RoomID)
            }
        }
    }
}

// ReadPump читает сообщения от браузера и отправляет в Hub
func (c *Client) ReadPump() {
    defer func() {
        log.Printf("ReadPump: Завершение для клиента %s", c.Username)
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
                log.Printf("WebSocket error для %s: %v", c.Username, err)
            } else {
                log.Printf("ReadPump: Нормальное закрытие соединения для %s: %v", c.Username, err)
            }
            break
        }

        // Заполняем метаданные сообщения
        msg.Username = c.Username
        msg.RoomID = c.RoomID
        msg.Timestamp = time.Now()

        if msg.Type == models.MessageTypeChat && c.Store != nil {
            err := c.Store.SaveMessage(storage.Message{
                Username: msg.Username,
                Content:  msg.Content,
            })
            if err != nil {
                log.Printf("Ошибка сохранения сообщения в БД: %v", err)
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
            log.Printf("WebRTC сообщение от %s: тип=%s, цель=%s", 
                c.Username, msg.Type, msg.TargetUser)
        } else {
            log.Printf("Получено сообщение от %s: тип=%s, содержимое=%s", 
                c.Username, msg.Type, msg.Content)
        }

        // Отправляем в Hub для рассылки
        select {
        case c.Hub.Broadcast <- msg:
            // Сообщение отправлено в Hub
        default:
            log.Printf("КРИТИЧЕСКАЯ ОШИБКА: Hub.Broadcast переполнен! Сообщение от %s потеряно", c.Username)
        }
    }
}

// WritePump отправляет сообщения из канала Send в браузер
func (c *Client) WritePump() {
    ticker := time.NewTicker(54 * time.Second)  // Ping каждые 54 секунды
    defer func() {
        ticker.Stop()
        c.Conn.Close()
        log.Printf("WritePump: Завершение для клиента %s", c.Username)
    }()

    log.Printf("WritePump запущен для клиента %s", c.Username)

    for {
        select {
        case message, ok := <-c.Send:
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            
            if !ok {
                // Hub закрыл канал - отправляем close message
                log.Printf("WritePump: Канал Send закрыт для %s", c.Username)
                c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            // ЛОГИРУЕМ WEBRTC ОТПРАВКУ
            if message.Type == models.MessageTypeWebRTCOffer || 
               message.Type == models.MessageTypeWebRTCAnswer || 
               message.Type == models.MessageTypeWebRTCCandidate {
                log.Printf("WritePump: Отправляем WebRTC сообщение %s клиенту %s", 
                    message.Type, c.Username)
            }

            // Отправляем JSON сообщение в браузер
            if err := c.Conn.WriteJSON(message); err != nil {
                log.Printf("WritePump: Ошибка отправки сообщения клиенту %s: %v", c.Username, err)
                return
            }

        case <-ticker.C:
            // Отправляем ping для поддержания соединения
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                log.Printf("WritePump: Ошибка ping для %s: %v", c.Username, err)
                return
            }
        }
    }
}

<<<<<<< HEAD
=======
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

// Отправить сообщение конкретному пользователю - С УЛУЧШЕННОЙ ОБРАБОТКОЙ ОШИБОК
func (h *Hub) SendToUser(message models.Message) {
    targetClient := h.FindClient(message.RoomID, message.TargetUser)
    if targetClient == nil {
        log.Printf("Hub: Пользователь %s не найден в комнате %s", message.TargetUser, message.RoomID)
        return
    }
    
    select {
    case targetClient.Send <- message:
        log.Printf("Hub: WebRTC сообщение %s отправлено пользователю %s", message.Type, message.TargetUser)
    default:
        log.Printf("Hub: Очередь пользователя %s переполнена, отключаем", message.TargetUser)
        close(targetClient.Send)
        delete(h.Clients[message.RoomID], targetClient)
    }
}

>>>>>>> b7106ae (added postgreSQL, updated UI)
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
        log.Printf("Ошибка сериализации списка пользователей: %v", err)
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
            log.Printf("Hub: Не удалось отправить users_list - Broadcast переполнен")
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
            log.Printf("Hub: Не удалось отправить async сообщение - Broadcast переполнен")
        }
    }()
}

func (h *Hub) Stop() {
    log.Println("Hub: Завершение работы через context")
    h.cancel()
}

func (h *Hub) shutdown() {
    log.Println("Hub: Завершаем соединения...")
    for _, clients := range h.Clients {
        for client := range clients {
            close(client.Send)
            client.Conn.Close()
        }
    }
}