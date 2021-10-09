package main

import (
	"os"

	"github.com/pion/webrtc/v3"
)

func main() {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	peerConn, err := webrtc.NewPeerConnection(config)

	if err != nil {
		panic(err)
	}
	defer peerConn.Close()

	var pipe PipingServerPipe
	if len(os.Args) > 1 {
		pipe = NewPipeWithSendURL(os.Args[1])
	} else {
		pipe = NewPipeWithServer("ppng.io")
	}
	go func() {
		arr := make([]byte, 1024)
		for {
			n, err := pipe.Reader.Read(arr)
			if err != nil {
				panic(err)
			}
			os.Stdout.Write(arr[:n])
		}
	}()
	arr := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(arr)
		if err != nil {
			panic(err)
		}
		pipe.Writer.Write(arr[:n])
	}
}
