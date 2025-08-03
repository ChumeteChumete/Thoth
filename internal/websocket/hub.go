package websocket

import (
    "log"
    "net/http"
    "time"
    
    "github.com/gorilla/websocket"
    "Thoth/internal/models"
)

// Upgrader превращает обычный HTTP запрос в WebSocket соединение
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true // Разрешаем подключения с любых доменов (для разработки)
    },
}

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
    for {
        select {
        case client := <-h.Register:
            // Новый клиент подключился
            if h.Clients[client.RoomID] == nil {
                h.Clients[client.RoomID] = make(map[*Client]bool)
            }
            h.Clients[client.RoomID][client] = true
            log.Printf("Клиент %s подключился к комнате %s", client.Username, client.RoomID)

        case client := <-h.Unregister:
            // Клиент отключился
            if clients, ok := h.Clients[client.RoomID]; ok {
                if _, ok := clients[client]; ok {
                    delete(h.Clients[client.RoomID], client)
                    close(client.Send)
                    log.Printf("Клиент %s отключился от комнаты %s", client.Username, client.RoomID)
                }
            }

        case message := <-h.Broadcast:
            // Пришло сообщение для рассылки
            if clients, ok := h.Clients[message.RoomID]; ok {
                for client := range clients {
                    select {
                    case client.Send <- message:
                        // Сообщение успешно отправлено в очередь клиента
                    default:
                        // Очередь клиента переполнена, отключаем его
                        close(client.Send)
                        delete(h.Clients[message.RoomID], client)
                    }
                }
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