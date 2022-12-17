package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	var dataChanWG sync.WaitGroup
	dataChanWG.Add(1)
	go func() {
		for {
			lineBytes, isPrefix, err := pipe.Reader.ReadLine()
			if err == io.EOF {
				break
			}
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
			case "datachannel-ready":
				dataChanWG.Done()
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
	var wg sync.WaitGroup
	wg.Add(2)
	dataChan.OnClose(func() {
		writeToStderr("[p2ping] webrtc.datachannel: close\n")
	})
	dataChan.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			switch string(msg.Data) {
			case "close":
				os.Stdout.Close()
				wg.Done()
				dataChan.SendText("close-accept")
			case "close-accept":
				wg.Done()
			}
		} else {
			os.Stdout.Write(msg.Data)
		}
	})
	pipe.WriteMessage("datachannel-ready", "ready")
	dataChanWG.Wait()
	dataChan.SetBufferedAmountLowThreshold(8 * 1024 * 1024)
	arr := make([]byte, 4096)
	c := make(chan struct{})
	dataChan.OnBufferedAmountLow(func() {
		c <- struct{}{}
	})
	for {
		if dataChan.BufferedAmount() > 16*1024*1024 {
			<-c
		}
		n, err := os.Stdin.Read(arr)
		if n > 0 {
			dataChan.Send(arr[:n])
		}
		if err != nil {
			if err == io.EOF {
				dataChan.SendText("close")
				break
			}
			panic(err)
		}
	}
	wg.Wait()
	dataChan.Close()
}
