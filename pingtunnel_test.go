package pingtunnel

import (
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/icmp"
)

func TestMyMsgSerialization(t *testing.T) {
	my := &MyMsg{
		Id:     "12345",
		Target: "111:11",
		Type:   12,
		Data:   make([]byte, 0),
	}
	dst, err := proto.Marshal(my)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	my1 := &MyMsg{}
	if err := proto.Unmarshal(dst, my1); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if my1.Id != my.Id || my1.Target != my.Target || my1.Type != my.Type || len(my1.Data) != 0 {
		t.Errorf("Unmarshaled data mismatch: got %v, want %v", my1, my)
	}
}

func TestHighLoadPacketProcessing(t *testing.T) {
	conn, err := icmp.ListenPacket("ip4:icmp", "")
	if err != nil {
		t.Skipf("ICMP listen failed, may require root: %v", err)
	}
	defer conn.Close()

	recv := make(chan *Packet, 1000)
	var exit bool
	var wg sync.WaitGroup
	go recvICMP(&wg, &exit, *conn, recv)

	serverAddr, _ := net.ResolveIPAddr("ip4", "127.0.0.1")
	data := []byte("testdata")
	for i := 0; i < 1000; i++ {
		sendICMP(i%65535, i, *conn, serverAddr, "", "conn"+string(i), uint32(MyMsg_DATA), data,
			8, -1, 0, 0, 256*1024, 5000, 400, 0, 0, 60)
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	startAlloc := memStats.Alloc

	time.Sleep(time.Second)
	select {
	case p := <-recv:
		if p.my.Type != int32(MyMsg_DATA) || string(p.my.Data) != "testdata" {
			t.Errorf("Received packet mismatch: got %v", p.my)
		}
	default:
	}

	runtime.ReadMemStats(&memStats)
	if memStats.Alloc-startAlloc > 10*1024*1024 {
		t.Errorf("Memory usage too high: %d bytes", memStats.Alloc-startAlloc)
	}

	exit = true
	wg.Wait()
}
