package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/pion/webrtc/v2"
)

func TestRTC(t *testing.T) {
	srv := rtcServer{
		Config: &webrtc.Configuration{},
	}
	srv.HandleConnection = func(conn rtcConn) {
		t.Log("Server: New rtc connection")
		conn.DataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
			t.Log("Server: Received message, sending echo")
			if err := conn.DataChannel.SendText("Echo: " + string(msg.Data)); err != nil {
				t.Error(err)
			}
		})
	}
	srv.init()
	api := srv.api

	// Setup fake handshake server
	ts := httptest.NewServer(&srv)
	defer ts.Close()

	// Prepare peer connection
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
	// Send peer handshake to the fake server
	client := ts.Client()
	encodedOffer, err := Encode(offerSession)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Peer: Sending offer", encodedOffer)
	res, err := client.Get(ts.URL + "?offer=" + encodedOffer)
	if err != nil {
		t.Fatal(err)
	}
	var answerSession webrtc.SessionDescription
	if err := json.NewDecoder(res.Body).Decode(&answerSession); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	t.Log("Peer: Received answer, setting remote session", answerSession)
	if err := offerPC.SetRemoteDescription(answerSession); err != nil {
		t.Fatal(err)
	}

	// Request a data channel, must happen after session is established
	offerDC, err := offerPC.CreateDataChannel("test-data-channel", nil)
	if err != nil {
		t.Fatal(err)
	}
	offerDC.OnOpen(func() {
		t.Log("offerDC.OnOpen")
	})

	t.Log("Peer: Waiting for data channel to open")
	open := make(chan struct{})
	offerDC.OnOpen(func() {
		open <- struct{}{}
	})
	<-open
	t.Log("Peer: Data channel opened")

	received := make(chan []byte)
	offerDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		t.Log("offerDC.OnMessage", msg)
		received <- msg.Data
	})

	t.Log("Peer: Sending message to answerer")
	if err := offerDC.Send([]byte("Ping")); err != nil {
		t.Fatal(err)
	}

	t.Log("Peer: Waiting for response")
	if got, want := <-received, []byte("Echo: Ping"); !bytes.Equal(got, want) {
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
