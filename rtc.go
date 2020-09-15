package main

import (
	"context"
	"fmt"
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
	Peer        *webrtc.PeerConnection
	DataChannel *webrtc.DataChannel
}

type rtcServer struct {
	Logger zerolog.Logger
	Config *webrtc.Configuration

	conns map[uint16]conn
}

func (s *rtcServer) accept(config webrtc.Configuration, offer <-chan webrtc.SessionDescription) (*webrtc.PeerConnection, error) {
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})

	// Register data channel creation handling
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

		// Register channel opening handling
		d.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open. Random messages will now be sent to any connected DataChannels every 5 seconds\n", d.Label(), d.ID())

			for range time.NewTicker(5 * time.Second).C {
				message := "hi"
				fmt.Printf("Sending '%s'\n", message)

				// Send the message as text
				sendErr := d.SendText(message)
				if sendErr != nil {
					panic(sendErr)
				}
			}
		})

		// Register text message handling
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			fmt.Printf("Message from DataChannel '%s': '%s'\n", d.Label(), string(msg.Data))
		})
	})

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(<-offer)
	if err != nil {
		return nil, err
	}

	// Create an answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		return nil, err
	}

	// XXX: Incomplete
	return peerConnection
}

func (s *rtcServer) Serve(ctx context.Context) error {
	// Create a new RTCPeerConnection

	// Block forever
	select {}
}
