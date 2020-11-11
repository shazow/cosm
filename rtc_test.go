package main

import (
	"bytes"
	"testing"

	"github.com/pion/webrtc/v2"
)

func TestRTC(t *testing.T) {
	srv := rtcServer{
		Config: &webrtc.Configuration{},
	}
	srv.init()
	api := srv.api

	offerPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer offerPC.Close()

	offerSession, err := prepareOffer(offerPC)
	if err != nil {
		t.Fatal("failed to prepare offer", err)
	}

	offerPC.OnDataChannel(func(d *webrtc.DataChannel) {
		t.Log("offerPC.OnDataChannel", d)
	})
	offerDC, err := offerPC.CreateDataChannel("test-data-channel", nil)
	if err != nil {
		t.Fatal(err)
	}
	offerDC.OnOpen(func() {
		t.Log("offerDC.OnOpen")
	})

	// rtcServer.accept handles:
	// * conn := NewPeerConnection
	// * conn.SetRemoteDescription(offer)
	// * answer := conn.CreateAnswer(nil)
	// * conn.SetLocalDescription(answer)
	rtcConn, err := srv.accept(*offerSession)
	if err != nil {
		t.Fatal("failed to accept rtcServer offer", err)
	}
	defer rtcConn.Close()

	rtcConn.OnDataChannel(func(dc *webrtc.DataChannel) {
		t.Log("rtcConn.OnDataChannel", dc)
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			t.Log("rtcConn.dc.OnMessage", dc)
			dc.Send([]byte("Pong"))
		})
	})

	// Acquire answer
	answer := rtcConn.LocalDescription()
	if err = offerPC.SetRemoteDescription(*answer); err != nil {
		t.Fatal(err)
	}

	t.Log("offer: ", offerSession)
	t.Log("answer: ", answer)

	t.Log("Waiting for data channel to open")
	open := make(chan struct{})
	offerDC.OnOpen(func() {
		open <- struct{}{}
	})
	<-open
	t.Log("data channel opened")

	received := make(chan []byte)
	offerDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		t.Log("offerDC.OnMessage", msg)
		received <- msg.Data
	})

	if err := offerDC.Send([]byte("Ping")); err != nil {
		t.Fatal(err)
	}

	t.Log("waiting for pong")
	if got, want := <-received, []byte("Pong"); !bytes.Equal(got, want) {
		t.Errorf("got: %s; want: %s", got, want)
	} else {
		t.Logf("received response: %s", got)
	}
}

// prepareOffer sets up the signal of the offer side of the peer connection.
//
// Based on https://github.com/pion/webrtc/blob/v2/peerconnection_test.go#L31
//
// Note: If this ever needs to run in js/wasm, we'll need to create a dummy
// datachannel to trigger ICE gathering.
func prepareOffer(pc *webrtc.PeerConnection) (*webrtc.SessionDescription, error) {
	iceGatheringState := pc.ICEGatheringState()
	offerCh := make(chan *webrtc.SessionDescription, 1)

	if iceGatheringState != webrtc.ICEGatheringStateComplete {
		pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate == nil {
				offerCh <- pc.PendingLocalDescription()
			}
		})
	}

	if offer, err := pc.CreateOffer(nil); err != nil {
		return nil, err
	} else if err := pc.SetLocalDescription(offer); err != nil {
		return nil, err
	} else if iceGatheringState == webrtc.ICEGatheringStateComplete {
		return &offer, nil
	}

	return <-offerCh, nil
}
