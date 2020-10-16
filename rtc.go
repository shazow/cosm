package main

import (
	"context"
	"encoding/json"
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

	HandleConnection func(conn)

	newConn chan conn
	conns   map[uint16]conn
}

func (s *rtcServer) init() {
	engine := webrtc.SettingEngine{}
	engine.DetachDataChannels()
	s.api = webrtc.NewAPI(webrtc.WithSettingEngine(engine))

	if s.Config == nil {
		s.Config = &defaultWebRTCConfig
	}
}

// accept takes a SessionDescription offer and returns a PeerConnection with a
// LocalDescription answer that has not been accepted remotely yet.
// The client must accept the PeerConnection.LocalDescription() for the
// connection to complete.
func (s *rtcServer) accept(offer webrtc.SessionDescription) (*webrtc.PeerConnection, error) {
	// Create a new RTCPeerConnection using the API object
	peerConnection, err := s.api.NewPeerConnection(*s.Config)
	if err != nil {
		return nil, err
	}

	// Set the remote SessionDescription (offer received from peer)
	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	// Create an answer (sent back to the peer)
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
	if s.HandleConnection == nil {
		http.Error(w, "not set to handle new connections", http.StatusInternalServerError)
		return
	}

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
		s.HandleConnection(conn{
			Peer:        peerConn,
			DataChannel: d,
		})
	})
}
