package models

import "time"

type Message struct {
    Type      string    `json:"type"`
    ID        int       `json:"id"`
    Username  string    `json:"username"`
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
    RoomID    string    `json:"room_id"`
    TargetUser string      `json:"target_user,omitempty"`
    WebRTCData interface{} `json:"webrtc_data,omitempty"`
}

const (
    MessageTypeChat         = "chat"
    MessageTypeUserJoined   = "user_joined"
    MessageTypeUserLeft     = "user_left"
    MessageTypeUsersList    = "users_list"
    MessageTypeWebRTCOffer     = "webrtc_offer"
    MessageTypeWebRTCAnswer    = "webrtc_answer"
    MessageTypeWebRTCCandidate = "webrtc_candidate"
)

type User struct {
    Username string `json:"username"`
    RoomID   string `json:"room_id"`
}