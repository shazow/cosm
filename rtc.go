package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pion/webrtc/v2"
	"github.com/rs/zerolog"
)

var defaultWebRTCConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type conn struct {
	io.ReadWriteCloser

	Peer        *webrtc.PeerConnection
	DataChannel *webrtc.DataChannel
}

func (c *conn) open() (err error) {
	c.ReadWriteCloser, err = c.DataChannel.Detach()
	return err
}

type rtcServer struct {
	Logger zerolog.Logger
	Config *webrtc.Configuration
	api    *webrtc.API

	newConn chan conn
	conns   map[uint16]conn
}

// accept takes a SessionDescription offer and returns a PeerConnection with a LocalDescription answer that has not been accepted remotely yet.
// The client must accept the PeerConnection.LocalDescription() for the connection to complete.
func (s *rtcServer) accept(offer webrtc.SessionDescription) (*webrtc.PeerConnection, error) {
	cfg := defaultWebRTCConfig
	if s.Config != nil {
		cfg = *s.Config
	}

	// Create a new RTCPeerConnection using the API object
	peerConnection, err := s.api.NewPeerConnection(cfg)
	if err != nil {
		return nil, err
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})

	// Register data channel creation handling
	/*
		peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
			s.Logger.Debug().Str("label", d.Label()).Interface("id", d.ID()).Msg("new datachannel")
			newConn <- conn{
				Peer:        peerConnection,
				DataChannel: d,
			}
		})
	*/

	// Set the remote SessionDescription
	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	// Create an answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	// In production we should exchange ICE Candidates via OnICECandidate
	// Example: https://github.com/pion/webrtc/blob/v2/examples/data-channels-flow-control/main.go

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		return nil, err
	}

	return peerConnection, nil
}

func (s *rtcServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var offer webrtc.SessionDescription
	if err := Decode(r.FormValue("offer"), &offer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	peerConn, err := s.accept(offer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	w.Header().Set("content-type", "application/json")

	answer := peerConn.LocalDescription()
	if err := json.NewEncoder(w).Encode(answer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.Canceled {
			return
		}
		peerConn.Close()
	}()

	peerConn.OnDataChannel(func(d *webrtc.DataChannel) {
		cancel()
		if ctx.Err() != context.Canceled {
			return
		}
		s.Logger.Debug().Str("label", d.Label()).Interface("id", d.ID()).Msg("new datachannel")
		s.newConn <- conn{
			Peer:        peerConn,
			DataChannel: d,
		}
	})
}

func (s *rtcServer) Serve(ctx context.Context) error {
	// Create a SettingEngine and enable Detach
	engine := webrtc.SettingEngine{}
	engine.DetachDataChannels()

	// Create an API object with the engine
	s.api = webrtc.NewAPI(webrtc.WithSettingEngine(engine))

	for {
		//	s.accept()
	}
}
