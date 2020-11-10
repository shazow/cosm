package main

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/pion/webrtc/v2"
)

func TestRTC(t *testing.T) {
	srv := rtcServer{}
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

	ready := make(chan struct{})

	offerPC.OnDataChannel(func(d *webrtc.DataChannel) {
		t.Log("offerPC.OnDataChannel", d)
	})
	offerDC, err := offerPC.CreateDataChannel("test-data-channel", nil)
	if err != nil {
		t.Fatal(err)
	}
	offerDC.OnOpen(func() {
		t.Log("offerDC.OnOpen")
		ready <- struct{}{}
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

	// Acquire answer
	answer := rtcConn.LocalDescription()
	if err = offerPC.SetRemoteDescription(*answer); err != nil {
		t.Fatal(err)
	}
	t.Log("offer: ", offerSession)
	t.Log("answer: ", answer)

	t.Log("waiting for data channel")
	rtcConn.OnDataChannel(func(dc *webrtc.DataChannel) {
		t.Log("rtcConn.OnDataChannel", dc)
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			t.Log("rtcConn.dc.OnMessage", dc)
			dc.Send([]byte("Pong"))
		})
	})

	received := make(chan []byte)
	offerDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		t.Log("offerDC.OnMessage", msg)
		received <- msg.Data
	})

	<-ready
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

// signalPair is borrowed from https://github.com/pion/webrtc/blob/v2/peerconnection_test.go
func signalPair(pcOffer *webrtc.PeerConnection, pcAnswer *webrtc.PeerConnection) error {
	iceGatheringState := pcOffer.ICEGatheringState()
	offerChan := make(chan webrtc.SessionDescription, 1)

	if iceGatheringState != webrtc.ICEGatheringStateComplete {
		pcOffer.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate == nil {
				offerChan <- *pcOffer.PendingLocalDescription()
			}
		})
	}
	// Note(albrow): We need to create a data channel in order to trigger ICE
	// candidate gathering in the background for the JavaScript/Wasm bindings. If
	// we don't do this, the complete offer including ICE candidates will never be
	// generated.
	if _, err := pcOffer.CreateDataChannel("initial_data_channel", nil); err != nil {
		return err
	}

	offer, err := pcOffer.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pcOffer.SetLocalDescription(offer); err != nil {
		return err
	}

	if iceGatheringState == webrtc.ICEGatheringStateComplete {
		offerChan <- offer
	}
	select {
	case <-time.After(3 * time.Second):
		return fmt.Errorf("timed out waiting to receive offer")
	case offer := <-offerChan:
		if err := pcAnswer.SetRemoteDescription(offer); err != nil {
			return err
		}

		answer, err := pcAnswer.CreateAnswer(nil)
		if err != nil {
			return err
		}

		if err = pcAnswer.SetLocalDescription(answer); err != nil {
			return err
		}

		err = pcOffer.SetRemoteDescription(answer)
		if err != nil {
			return err
		}
		return nil
	}
}
