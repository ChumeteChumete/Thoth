package websocket

import (
    "encoding/json"
    "log"
    "time"
    
    "github.com/gorilla/websocket"
    "Thoth/internal/models"
)

// Client представляет одного подключенного пользователя
type Client struct {
    Hub      *Hub                   // Ссылка на главный Hub
    Conn     *websocket.Conn        // WebSocket соединение
    Send     chan models.Message    // Канал для отправки сообщений этому клиенту
    Username string                 // Имя пользователя
    RoomID   string                 // В какой комнате находится
}

// Hub управляет всеми клиентами и сообщениями
type Hub struct {
    // Активные клиенты по комнатам
    Clients map[string]map[*Client]bool // [roomID][client] = active
    
    // Каналы для коммуникации
    Broadcast  chan models.Message  // Канал для рассылки сообщений
    Register   chan *Client         // Канал для регистрации новых клиентов  
    Unregister chan *Client         // Канал для отключения клиентов
}

// NewHub создает новый Hub
func NewHub() *Hub {
    return &Hub{
        Clients:    make(map[string]map[*Client]bool),
        Broadcast:  make(chan models.Message),
        Register:   make(chan *Client),
        Unregister: make(chan *Client),
    }
}

func (h *Hub) Run() {
    log.Println("Hub запущен и ожидает события")
    for {
        select {
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
            h.SendMessageAsync(joinMessage) // ИСПОЛЬЗУЕМ АСИНХРОННЫЙ МЕТОД

            // АСИНХРОННО отправляем список пользователей
            log.Printf("Hub: Отправляем список пользователей асинхронно")
            h.BroadcastUsersList(client.RoomID) // Уже асинхронный

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
                    h.SendMessageAsync(leaveMessage) // АСИНХРОННО
                    h.BroadcastUsersList(client.RoomID) // Уже асинхронный
                }
            }

        case message := <-h.Broadcast:
            log.Printf("Hub: Получено сообщение для рассылки: тип='%s', от='%s', комната='%s'", 
                message.Type, message.Username, message.RoomID)
                
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
            }
        }
    }
}

// ReadPump читает сообщения от браузера и отправляет в Hub
func (c *Client) ReadPump() {
    defer func() {
        c.Hub.Unregister <- c  // При выходе - отключаемся от Hub
        c.Conn.Close()         // Закрываем WebSocket соединение
    }()

    // Настройки соединения
    c.Conn.SetReadLimit(512)                    // Макс размер сообщения
    c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second)) // Таймаут чтения
    
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
                log.Printf("WebSocket error: %v", err)
            }
            break
        }

        // Заполняем метаданные сообщения
        msg.Username = c.Username
        msg.RoomID = c.RoomID
        msg.Timestamp = time.Now()

        // Если тип не указан, считаем что это чат
        if msg.Type == "" {
            msg.Type = models.MessageTypeChat
        }

        log.Printf("Получено сообщение от %s: тип=%s, содержимое=%s", 
            c.Username, msg.Type, msg.Content)

        // Отправляем в Hub для рассылки всем
        c.Hub.Broadcast <- msg
    }
}

// WritePump отправляет сообщения из канала Send в браузер
func (c *Client) WritePump() {
    ticker := time.NewTicker(54 * time.Second)  // Ping каждые 54 секунды
    defer func() {
        ticker.Stop()
        c.Conn.Close()
    }()

    log.Printf("WritePump запущен для клиента %s", c.Username)

    for {
        select {
        case message, ok := <-c.Send:
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            
            if !ok {
                // Hub закрыл канал - отправляем close message
                c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            // Отправляем JSON сообщение в браузер
            if err := c.Conn.WriteJSON(message); err != nil {
                log.Printf("Ошибка отправки сообщения: %v", err)
                return
            }

        case <-ticker.C:
            // Отправляем ping для поддержания соединения
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
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
        h.Broadcast <- usersMessage
    }()
}

// Метод для асинхронной отправки сообщений
func (h *Hub) SendMessageAsync(message models.Message) {
    go func() {
        h.Broadcast <- message
    }()
}