package main

import (
	"goatrobotics/services"
	"net/http"

	"github.com/sirupsen/logrus"
)

func main() {
	loggy := logrus.New()
	chatRoom := services.NewChatRoom(loggy)
	go chatRoom.Run()

	http.HandleFunc("/join", chatRoom.HandleJoin)
	http.HandleFunc("/leave", chatRoom.HandleLeave)
	http.HandleFunc("/send", chatRoom.HandleSend)
	http.HandleFunc("/messages", chatRoom.HandleMessages)

	loggy.Info("Starting chat server on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		loggy.Fatal("Server error:", err)
	}
}
