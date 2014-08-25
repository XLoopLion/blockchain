package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/izqui/helpers"
)

type ConnectionsQueue chan string
type NodeChannel chan *Node
type Node struct {
	*net.TCPConn
	lastSeen int
}

type Nodes map[string]*Node

type Network struct {
	Nodes
	ConnectionsQueue
	Address            string
	ConnectionCallback NodeChannel
	BroadcastQueue     chan Message
	IncomingMessages   chan Message
}

func (n Nodes) AddNode(node *Node) bool {

	key := node.TCPConn.RemoteAddr().String()

	if key != self.Address && n[key] == nil {

		fmt.Println("Node connected", key)
		n[key] = node

		go func() {
			for {
				var bs []byte = make([]byte, 1024*100)
				n, err := node.TCPConn.Read(bs[0:])
				networkError(err)

				if err == io.EOF {
					//TODO: Remove node [Issue: https://github.com/izqui/blockchain/issues/3]
					node.TCPConn.Close()

					break
				}

				m := new(Message)
				err = m.UnmarshalBinary(bs[0:n])

				if err != nil {
					fmt.Println(err)
					continue

				} else {

					m.Reply = make(chan Message)

					go func(cb chan Message) {
						for {
							m, ok := <-cb

							b, _ := m.MarshalBinary()
							l := len(b)

							i := 0
							for i < l {
								a, _ := node.TCPConn.Write(b[i:])
								i += a
							}

							if !ok {
								close(cb)
								break
							}
						}

					}(m.Reply)

					self.Network.IncomingMessages <- *m
				}
			}
		}()

		return true
	}
	return false
}

func SetupNetwork(address, port string) *Network {

	n := new(Network)

	n.BroadcastQueue, n.IncomingMessages = make(chan Message), make(chan Message)
	n.ConnectionsQueue, n.ConnectionCallback = CreateConnectionsQueue()
	n.Nodes = Nodes{}
	n.Address = fmt.Sprintf("%s:%s", address, port)

	return n
}

func (n *Network) Run() {

	fmt.Println("Listening in", self.Address)
	listenCb := StartListening(self.Address)

	for {
		select {
		case node := <-listenCb:

			self.Nodes.AddNode(node)
		case node := <-n.ConnectionCallback:

			self.Nodes.AddNode(node)
		case message := <-n.BroadcastQueue:
			go n.BroadcastMessage(message)

		case message := <-n.IncomingMessages:
			switch message.Identifier {
			case MESSAGE_SEND_TRANSACTION:
				t := new(Transaction)
				t.UnmarshalBinary(message.Data)
				self.Blockchain.TransactionsQueue <- t
			}
		}
	}

}

func CreateConnectionsQueue() (ConnectionsQueue, NodeChannel) {

	in := make(ConnectionsQueue)
	out := make(NodeChannel)

	go func() {

		for {
			address := <-in

			address = fmt.Sprintf("%s:%s", address, BLOCKCHAIN_PORT)

			if address != self.Address && self.Nodes[address] == nil {

				go ConnectToNode(address, 5*time.Second, false, out)
			}
		}
	}()

	return in, out
}

func StartListening(address string) NodeChannel {

	cb := make(NodeChannel)
	addr, err := net.ResolveTCPAddr("tcp4", address)
	networkError(err)

	listener, err := net.ListenTCP("tcp4", addr)
	networkError(err)

	go func(l *net.TCPListener) {

		for {
			connection, err := l.AcceptTCP()
			networkError(err)

			cb <- &Node{connection, int(time.Now().Unix())}
		}

	}(listener)

	return cb
}

func ConnectToNode(dst string, timeout time.Duration, retry bool, cb NodeChannel) {

	addrDst, err := net.ResolveTCPAddr("tcp4", dst)
	networkError(err)

	var con *net.TCPConn = nil
loop:
	for {
		breakChannel := make(chan bool)
		go func() {

			fmt.Println("Attempting to connect to", dst)
			con, err = net.DialTCP("tcp", nil, addrDst)

			if con != nil {

				cb <- &Node{con, int(time.Now().Unix())}
				breakChannel <- true
			}
		}()

		select {
		case <-helpers.Timeout(timeout):
			if !retry {
				break loop
			}
		case <-breakChannel:
			break loop
		}

	}
}

func (n *Network) BroadcastMessage(message Message) {

	b, _ := message.MarshalBinary()
	l := len(b)
	for _, node := range n.Nodes {
		fmt.Println("broadcast", node.TCPConn.RemoteAddr())
		go func() {
			i := 0
			for i < l {
				a, _ := node.TCPConn.Write(b[i:])
				i += a
			}
		}()
	}
}

func GetIpAddress() []string {

	name, err := os.Hostname()
	if err != nil {

		return nil
	}

	addrs, err := net.LookupHost(name)
	if err != nil {

		return nil
	}

	return addrs
}

func networkError(err error) {

	if err != nil && err != io.EOF {

		log.Println("Blockchain network: ", err)
	}
}
