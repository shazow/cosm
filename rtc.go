package main

import (
	"context"
	"fmt"
	"io"

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

	conns map[uint16]conn
}

func (s *rtcServer) accept(api *webrtc.API, config webrtc.Configuration, offers <-chan webrtc.SessionDescription, newConn chan<- conn) (*webrtc.PeerConnection, error) {
	// Create a new RTCPeerConnection using the API object
	peerConnection, err := api.NewPeerConnection(config)
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
		s.Logger.Debug().Str("label", d.Label()).Interface("id", d.ID()).Msg("new datachannel")
		newConn <- conn{
			Peer:        peerConnection,
			DataChannel: d,
		}
	})

	// Create an offer to send to the browser
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		return nil, err
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Output the offer in base64 so we can paste it in browser
	fmt.Println(signal.Encode(*peerConnection.LocalDescription()))

	// Wait for the answer to be pasted
	answer := webrtc.SessionDescription{}
	signal.Decode(signal.MustReadStdin(), &answer)

	// Apply the answer as the remote description
	err = peerConnection.SetRemoteDescription(answer)

}

func (s *rtcServer) Serve(ctx context.Context) error {

	// Create a SettingEngine and enable Detach
	s := webrtc.SettingEngine{}
	s.DetachDataChannels()

	// Create an API object with the engine
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	config := defaultWebRTCConfig
	if s.Config != nil {
		config = *s.Config
	}

	for {
		//	s.accept()
	}
}
