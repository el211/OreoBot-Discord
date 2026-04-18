package minecraft

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	packetTypeCommand  = 2
	packetTypeAuth     = 3
	packetTypeResponse = 0
)

type Client struct {
	addr     string
	password string
	conn     net.Conn
	mu       sync.Mutex
	reqID    int32
}

func NewClient(ip string, port int, password string) *Client {
	return &Client{
		addr:     fmt.Sprintf("%s:%d", ip, port),
		password: password,
	}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := net.DialTimeout("tcp", c.addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("rcon connect: %w", err)
	}
	c.conn = conn

	resp, err := c.sendPacketLocked(packetTypeAuth, c.password)
	if err != nil {
		c.conn.Close()
		c.conn = nil
		return fmt.Errorf("rcon auth send: %w", err)
	}
	if resp.ID == -1 {
		c.conn.Close()
		c.conn = nil
		return errors.New("rcon auth failed: invalid password")
	}
	return nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) Command(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return "", errors.New("not connected")
	}

	resp, err := c.sendPacketLocked(packetTypeCommand, cmd)
	if err != nil {

		c.conn.Close()
		c.conn = nil
		if reconnErr := c.reconnectLocked(); reconnErr != nil {
			return "", fmt.Errorf("rcon reconnect failed: %w (original: %v)", reconnErr, err)
		}
		resp, err = c.sendPacketLocked(packetTypeCommand, cmd)
		if err != nil {
			return "", err
		}
	}
	return resp.Body, nil
}

func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}
