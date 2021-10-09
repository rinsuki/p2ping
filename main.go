package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"

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
	var host bool
	if len(os.Args) > 1 {
		host = false
		pipe = NewPipeAsClient(os.Args[1])
	} else {
		host = true
		pipe = NewPipeAsServer("ppng.io")
	}
	var pendingCandidates = []string{}
	var dataChan *webrtc.DataChannel
	go func() {
		for {
			lineBytes, isPrefix, err := pipe.Reader.ReadLine()
			// TODO: handle isPrefix
			if err != nil || isPrefix {
				panic(err)
			}
			line := bytes.SplitN(lineBytes, []byte("|"), 2)
			typ := string(line[0])
			body := line[1]
			// _, err = os.Stderr.WriteString(fmt.Sprintf("%s: %s\n", typ, body))
			// if err != nil {
			// 	panic(err)
			// }
			switch typ {
			case "sdp":
				sdp := webrtc.SessionDescription{}
				if sdpErr := json.Unmarshal(body, &sdp); sdpErr != nil {
					panic(sdpErr)
				}

				if sdpErr := peerConn.SetRemoteDescription(sdp); sdpErr != nil {
					panic(sdpErr)
				}

				if sdp.Type == webrtc.SDPTypeOffer {
					answer, err := peerConn.CreateAnswer(nil)
					if err != nil {
						panic(err)
					}
					if err = peerConn.SetLocalDescription(answer); err != nil {
						panic(err)
					}
					payload, err := json.Marshal(answer)
					if err != nil {
						panic(err)
					}
					pipe.WriteMessage("sdp", string(payload))
				}

				for _, candidate := range pendingCandidates {
					peerConn.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
				}
			case "icecandy":
				if peerConn.RemoteDescription() == nil {
					pendingCandidates = append(pendingCandidates, string(body))
					return
				}
				if iceErr := peerConn.AddICECandidate(webrtc.ICECandidateInit{Candidate: string(body)}); iceErr != nil {
					panic(iceErr)
				}
			}
		}
	}()
	peerConn.OnICECandidate(func(ice *webrtc.ICECandidate) {
		if ice == nil {
			return
		}
		pipe.WriteMessage("icecandy", ice.ToJSON().Candidate)
	})
	peerConn.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		payload, err := json.Marshal(pcs)
		if err != nil {
			panic(err)
		}
		writeToStderr(fmt.Sprintf("[p2ping] webrtc: state changed to %s\n", pcs.String()))
		if err != nil {
			panic(err)
		}
		pipe.WriteMessage("state", string(payload))
	})
	if host {
		var isOrdered = true
		dc, err := peerConn.CreateDataChannel("pipe", &webrtc.DataChannelInit{
			Ordered: &isOrdered,
		})
		dataChan = dc
		if err != nil {
			panic(err)
		}

		offer, err := peerConn.CreateOffer(nil)
		if err != nil {
			panic(err)
		}
		if err = peerConn.SetLocalDescription(offer); err != nil {
			panic(err)
		}
		payload, err := json.Marshal(offer)
		if err != nil {
			panic(err)
		}
		pipe.WriteMessage("sdp", string(payload))
	} else {
		var wg sync.WaitGroup
		wg.Add(1)
		peerConn.OnDataChannel(func(dc *webrtc.DataChannel) {
			dataChan = dc
			wg.Done()
		})
		wg.Wait()
	}
	dataChan.OnMessage(func(msg webrtc.DataChannelMessage) {
		os.Stdout.Write(msg.Data)
	})
	arr := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(arr)
		if err != nil {
			panic(err)
		}
		dataChan.Send(arr[:n])
	}
}
