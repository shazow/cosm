package main

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/pion/webrtc/v2"
)

func TestRTC(t *testing.T) {
	s := webrtc.SettingEngine{}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	offerPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}

	answerPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}

	defer closePairNow(t, offerPC, answerPC)

	answerPC.OnDataChannel(func(d *webrtc.DataChannel) {
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			e := d.Send([]byte("Pong"))
			if e != nil {
				t.Fatalf("Failed to send string on data channel")
			}
		})
		t.Log("answerPC.OnDataChannel", d)
	})

	offerDC, err := offerPC.CreateDataChannel("foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := signalPair(offerPC, answerPC); err != nil {
		t.Fatal(err)
	}

	t.Log("Waiting for data channel to open")
	open := make(chan struct{})
	offerDC.OnOpen(func() {
		open <- struct{}{}
	})
	<-open
	t.Log("data channel opened")

	received := make(chan []byte)
	offerDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		received <- msg.Data
	})
	if err := offerDC.Send([]byte("Ping")); err != nil {
		t.Fatal(err)
	}

	if got, want := <-received, []byte("Pong"); !bytes.Equal(got, want) {
		t.Errorf("got: %s; want: %s", got, want)
	} else {
		t.Logf("received response: %s", got)
	}

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

func closePairNow(t *testing.T, pc1, pc2 io.Closer) {
	var fail bool
	if err := pc1.Close(); err != nil {
		t.Errorf("Failed to close PeerConnection: %v", err)
		fail = true
	}
	if err := pc2.Close(); err != nil {
		t.Errorf("Failed to close PeerConnection: %v", err)
		fail = true
	}
	if fail {
		t.FailNow()
	}
}
