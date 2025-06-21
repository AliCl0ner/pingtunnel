package pingtunnel

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/esrrhs/gohome/common"
	"github.com/esrrhs/gohome/loggo"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 2048)
	},
}

func sendICMP(id int, sequence int, conn icmp.PacketConn, server *net.IPAddr, target string,
	connId string, msgType uint32, data []byte, sproto int, rproto int, key int,
	tcpmode int, tcpmode_buffer_size int, tcpmode_maxwin int, tcpmode_resend_time int, tcpmode_compress int, tcpmode_stat int,
	timeout int) {

	m := &MyMsg{
		Id:                  connId,
		Type:                int32(msgType),
		Target:              target,
		Data:                data,
		Rproto:              int32(rproto),
		Key:                 int32(key),
		Tcpmode:             int32(tcpmode),
		TcpmodeBuffersize:   int32(tcpmode_buffer_size),
		TcpmodeMaxwin:       int32(tcpmode_maxwin),
		TcpmodeResendTimems: int32(tcpmode_resend_time),
		TcpmodeCompress:     int32(tcpmode_compress),
		TcpmodeStat:         int32(tcpmode_stat),
		Timeout:             int32(timeout),
		Magic:               int32(MyMsg_MAGIC),
	}

	mb, err := proto.Marshal(m)
	if err != nil {
		loggo.Error("sendICMP Marshal MyMsg error %s %s", server.String(), err)
		return
	}

	body := &icmp.Echo{
		ID:   id,
		Seq:  sequence,
		Data: mb,
	}

	msg := &icmp.Message{
		Type: ipv4.ICMPType(sproto),
		Code: 0,
		Body: body,
	}

	bytes, err := msg.Marshal(nil)
	if err != nil {
		loggo.Error("sendICMP Marshal error %s %s", server.String(), err)
		return
	}

	conn.WriteTo(bytes, server)
}

func recvICMP(workResultLock *sync.WaitGroup, exit *bool, conn icmp.PacketConn, recv chan<- *Packet) {
	defer common.CrashLog()

	workResultLock.Add(1)
	defer workResultLock.Done()

	for !*exit {
		bytes := bufferPool.Get().([]byte)
		conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
		n, srcaddr, err := conn.ReadFrom(bytes)

		if err != nil {
			nerr, ok := err.(net.Error)
			if !ok || !nerr.Timeout() {
				loggo.Error("Error read icmp message %s", err)
			}
			bufferPool.Put(bytes)
			continue
		}

		if n <= 0 {
			bufferPool.Put(bytes)
			continue
		}

		echoId := int(binary.BigEndian.Uint16(bytes[4:6]))
		echoSeq := int(binary.BigEndian.Uint16(bytes[6:8]))

		my := &MyMsg{}
		err = proto.Unmarshal(bytes[8:n], my)
		if err != nil {
			loggo.Warn("Unmarshal MyMsg error: %s", err)
			bufferPool.Put(bytes)
			continue
		}

		if my.Magic != int32(MyMsg_MAGIC) {
			loggo.Warn("processPacket data invalid %s", my.Id)
			bufferPool.Put(bytes)
			continue
		}

		recv <- &Packet{my: my, src: srcaddr.(*net.IPAddr), echoId: echoId, echoSeq: echoSeq}
		bufferPool.Put(bytes)
	}
}

type Packet struct {
	my      *MyMsg
	src     *net.IPAddr
	echoId  int
	echoSeq int
}

const (
	FRAME_MAX_SIZE = 888
	FRAME_MAX_ID   = 1000000
)
