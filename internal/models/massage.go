package models

import "time"

type Message struct {
    ID        int       `json:"id"`
    Username  string    `json:"username"`
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
    RoomID    string    `json:"room_id"`
}

type User struct {
    Username string `json:"username"`
    RoomID   string `json:"room_id"`
}