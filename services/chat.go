package services

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// chat message
type Message struct {
	SenderID string
	Content  string
	Time     time.Time
}

// chat client
type Client struct {
	ID       string
	Messages chan Message
}

// ChatRoom for chat state and operations
type ChatRoom struct {
	clients   map[string]*Client
	broadcast chan Message
	join      chan *Client
	leave     chan string
	mutex     sync.RWMutex
	loggy     *logrus.Logger
}

func NewChatRoom(logger *logrus.Logger) *ChatRoom {
	return &ChatRoom{
		clients:   make(map[string]*Client),
		broadcast: make(chan Message),
		join:      make(chan *Client),
		leave:     make(chan string),
		loggy:     logger,
	}
}

func (cr *ChatRoom) Run() {
	for {
		select {
		case client := <-cr.join:
			cr.mutex.Lock()
			cr.clients[client.ID] = client
			cr.mutex.Unlock()
			cr.loggy.Info("Client joined the chat", client.ID)

		case clientID := <-cr.leave:
			cr.mutex.Lock()
			if client, exists := cr.clients[clientID]; exists {
				close(client.Messages)
				delete(cr.clients, clientID)
			}
			cr.mutex.Unlock()
			cr.loggy.Info("Client left the chat", clientID)

		case msg := <-cr.broadcast:
			cr.mutex.RLock()
			for _, client := range cr.clients {
				// Use non-blocking send to prevent deadlock if client buffer is full
				select {
				case client.Messages <- msg:
				default:
					// Message dropped if client's buffer is full
					cr.loggy.Info("client buffer is full")
				}
			}
			cr.mutex.RUnlock()
		}
	}
}

func (cr *ChatRoom) HandleJoin(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("id")
	if clientID == "" {
		http.Error(w, "Client ID is required", http.StatusBadRequest)
		cr.loggy.Error("Client ID is required")
		return
	}

	cr.mutex.RLock()
	if _, exists := cr.clients[clientID]; exists {
		cr.mutex.RUnlock()
		http.Error(w, "Client ID already exists", http.StatusConflict)
		cr.loggy.Error("Client ID already exists")
		return
	}
	cr.mutex.RUnlock()

	client := &Client{
		ID:       clientID,
		Messages: make(chan Message, 100), // Buffer size of 100 messages
	}

	cr.join <- client
	cr.loggy.Info("joined the chat", clientID)
	response, err := json.Marshal(map[string]interface{}{
		"status":  "success",
		"message": "client joined the chat",
	})
	if err != nil {
		cr.loggy.Error("error in formatting response")
		http.Error(w, "error in parsing of response, please retry", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func (cr *ChatRoom) HandleLeave(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("id")
	if clientID == "" {
		http.Error(w, "Client ID is required", http.StatusBadRequest)
		cr.loggy.Error("Client ID is required")
		return
	}

	cr.mutex.RLock()
	if _, exists := cr.clients[clientID]; !exists {
		cr.mutex.RUnlock()
		http.Error(w, "Client not found", http.StatusNotFound)
		cr.loggy.Error("Client ID not found")
		return
	}
	cr.mutex.RUnlock()

	cr.leave <- clientID
	cr.loggy.Info("left the chat", clientID)
	response, err := json.Marshal(map[string]interface{}{
		"status":  "success",
		"message": "client left the chat",
	})
	if err != nil {
		cr.loggy.Error("error in formatting response")
		http.Error(w, "error in parsing response, let's try again", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func (cr *ChatRoom) HandleSend(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("id")
	message := r.URL.Query().Get("message")

	if clientID == "" || message == "" {
		http.Error(w, "Client ID and message are required", http.StatusBadRequest)
		cr.loggy.Error("Client ID or message are required", clientID)
		return
	}

	cr.mutex.RLock()
	if _, exists := cr.clients[clientID]; !exists {
		cr.mutex.RUnlock()
		http.Error(w, "Client not found", http.StatusNotFound)
		cr.loggy.Error("Client ID not found", clientID)
		return
	}
	cr.mutex.RUnlock()

	cr.broadcast <- Message{
		SenderID: clientID,
		Content:  message,
		Time:     time.Now(),
	}

	cr.loggy.Info("message sent")
	response, err := json.Marshal(map[string]interface{}{
		"status":  "success",
		"message": "message sent",
		"payload": message,
	})
	if err != nil {
		cr.loggy.Error("error in formatting response")
		http.Error(w, "error in parsing response, let's try again", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func (cr *ChatRoom) HandleMessages(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("id")
	if clientID == "" {
		http.Error(w, "Client ID is required", http.StatusBadRequest)
		cr.loggy.Error("Client ID is required", clientID)
		return
	}

	cr.mutex.RLock()
	client, exists := cr.clients[clientID]
	if !exists {
		cr.mutex.RUnlock()
		http.Error(w, "Client not found", http.StatusNotFound)
		cr.loggy.Error("Clinet ID not found", clientID)
		return
	}
	cr.mutex.RUnlock()

	select {
	case msg, ok := <-client.Messages:
		if !ok {
			http.Error(w, "Client channel closed", http.StatusGone)
			cr.loggy.Error("client channel is closed", clientID)
			return
		}
		cr.loggy.Info(msg.SenderID, msg.Content)
		response, err := json.Marshal(map[string]interface{}{
			"status":  "success",
			"client":  msg.SenderID,
			"message": msg.Content,
		})
		if err != nil {
			cr.loggy.Error("error in formatting response")
			http.Error(w, "error in parsing response, let's try again", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(response)
	case <-time.After(30 * time.Second):
		cr.loggy.Info("no new messages")
		cr.loggy.Info("message sent")
		response, err := json.Marshal(map[string]interface{}{
			"message": "now new messages",
		})
		if err != nil {
			cr.loggy.Error("error in formatting response")
			http.Error(w, "error in parsing response, let's try again", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(response)
	}
}
