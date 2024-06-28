package godis

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

type connection struct {
	host              string
	port              int
	connectionTimeout time.Duration
	soTimeout         time.Duration

	socket            net.Conn
	protocol          *protocol
	broken            bool
	pipelinedCommands int
}

func newConnection(host string, port int, connectionTimeout, soTimeout time.Duration) *connection {
	if host == "" {
		host = defaultHost
	}
	if port == 0 {
		port = defaultPort
	}
	if connectionTimeout == 0 {
		connectionTimeout = defaultTimeout
	}
	if soTimeout == 0 {
		soTimeout = defaultTimeout
	}
	return &connection{
		host:              host,
		port:              port,
		connectionTimeout: connectionTimeout,
		soTimeout:         soTimeout,
		broken:            false,
	}
}

func (c *connection) setTimeoutInfinite() error {
	if !c.isConnected() {
		err := c.connect()
		if err != nil {
			return err
		}
	}
	err := c.socket.SetDeadline(time.Time{})
	if err != nil {
		c.broken = true
		return newConnectError(err.Error())
	}
	return nil
}

func (c *connection) rollbackTimeout() error {
	if c.socket == nil {
		c.broken = true
		return newConnectError("socket is closed")
	}
	err := c.socket.SetDeadline(time.Now().Add(c.connectionTimeout))
	if err != nil {
		c.broken = true
		return newConnectError(err.Error())
	}
	return nil
}

func (c *connection) resetPipelinedCount() {
	c.pipelinedCommands = 0
}

func (c *connection) sendCommand(cmd protocolCommand, args ...[]byte) error {
	err := c.connect()
	if err != nil {
		return err
	}
	if err := c.protocol.sendCommand(cmd.getRaw(), args...); err != nil {
		return err
	}
	c.pipelinedCommands++
	return nil
}

func (c *connection) sendCommandByStr(cmd string, args ...[]byte) error {
	err := c.connect()
	if err != nil {
		return err
	}
	if err := c.protocol.sendCommand([]byte(cmd), args...); err != nil {
		return err
	}
	c.pipelinedCommands++
	return nil
}

func (c *connection) readProtocolWithCheckingBroken() (interface{}, error) {
	if c.broken {
		return nil, newConnectError("attempting to read from a broken connection")
	}
	read, err := c.protocol.read()
	if err == nil {
		return read, nil
	}
	switch err.(type) {
	case *ConnectError:
		c.broken = true
	}
	return nil, err
}

func (c *connection) getStatusCodeReply() (string, error) {
	reply, err := c.getOne()
	if err != nil {
		return "", err
	}
	if reply == nil {
		return "", nil
	}
	switch t := reply.(type) {
	case string:
		return t, nil
	case []byte:
		return string(t), nil
	default:
		return "", newDataError(fmt.Sprintf("data error:%v", reply))
	}
}

func (c *connection) getBulkReply() (string, error) {
	result, err := c.getBinaryBulkReply()
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (c *connection) getBinaryBulkReply() ([]byte, error) {
	reply, err := c.getOne()
	if err != nil {
		return nil, err
	}
	if reply == nil {
		return []byte{}, nil
	}
	switch reply.(type) {
	case []byte:
		return reply.([]byte), nil
	case []interface{}:
		arr := make([]byte, 0)
		for _, i := range reply.([]interface{}) {
			arr = append(arr, i.(byte))
		}
		return arr, nil
	}
	return reply.([]byte), nil
}

func (c *connection) getIntegerReply() (int64, error) {
	reply, err := c.getOne()
	if err != nil {
		return 0, err
	}
	if reply == nil {
		return 0, nil
	}
	switch reply.(type) {
	case int64:
		return reply.(int64), nil
	}
	return -1, nil
}

func (c *connection) getMultiBulkReply() ([]string, error) {
	reply, err := c.getBinaryMultiBulkReply()
	if err != nil {
		return nil, err
	}
	resp := make([]string, 0)
	for _, r := range reply {
		resp = append(resp, string(r))
	}
	return resp, nil
}

func (c *connection) getBinaryMultiBulkReply() ([][]byte, error) {
	reply, err := c.getOne()
	if err != nil {
		return nil, err
	}
	if reply == nil {
		return [][]byte{}, nil
	}
	resp := reply.([]interface{})
	arr := make([][]byte, 0)
	for _, res := range resp {
		arr = append(arr, res.([]byte))
	}
	return arr, nil
}

func (c *connection) getUnflushedObjectMultiBulkReply() ([]interface{}, error) {
	reply, err := c.readProtocolWithCheckingBroken()
	if err != nil {
		return nil, err
	}
	if reply == nil {
		return []interface{}{}, nil
	}
	return reply.([]interface{}), nil
}

func (c *connection) getRawObjectMultiBulkReply() ([]interface{}, error) {
	return c.getUnflushedObjectMultiBulkReply()
}

func (c *connection) getObjectMultiBulkReply() ([]interface{}, error) {
	if err := c.flush(); err != nil {
		return nil, err
	}
	c.pipelinedCommands--
	return c.getRawObjectMultiBulkReply()
}

func (c *connection) getIntegerMultiBulkReply() ([]int64, error) {
	reply, err := c.getOne()
	if err != nil {
		return nil, err
	}
	if reply == nil {
		return []int64{}, nil
	}
	switch reply.(type) {
	case []interface{}:
		arr := make([]int64, 0)
		for _, item := range reply.([]interface{}) {
			arr = append(arr, item.(int64))
		}
		return arr, nil
	default:
		return reply.([]int64), nil
	}
}

func (c *connection) getOne() (interface{}, error) {
	if err := c.flush(); err != nil {
		return "", err
	}
	c.pipelinedCommands--
	return c.readProtocolWithCheckingBroken()
}

func (c *connection) getAll(expect ...int) (interface{}, error) {
	num := 0
	if len(expect) > 0 {
		num = expect[0]
	}
	if err := c.flush(); err != nil {
		return nil, err
	}
	all := make([]interface{}, 0)
	for c.pipelinedCommands > num {
		obj, err := c.readProtocolWithCheckingBroken()
		if err != nil {
			all = append(all, err)
		} else {
			all = append(all, obj)
		}
		c.pipelinedCommands--
	}
	return all, nil
}

func (c *connection) flush() error {
	err := c.protocol.os.flush()
	if err != nil {
		c.broken = true
		return newConnectError(err.Error())
	}
	return nil
}

func (c *connection) connect() error {
	if c.isConnected() {
		return nil
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprint(c.host, ":", c.port), c.connectionTimeout)
	if err != nil {
		return newConnectError(err.Error())
	}
	err = conn.SetDeadline(time.Now().Add(c.soTimeout))
	if err != nil {
		return newConnectError(err.Error())
	}
	c.socket = conn
	os := newRedisOutputStream(bufio.NewWriter(c.socket), c)
	is := newRedisInputStream(bufio.NewReader(c.socket), c)
	c.protocol = newProtocol(os, is)
	return nil
}

func (c *connection) isConnected() bool {
	if c.socket == nil {
		return false
	}
	return true
}

func (c *connection) close() error {
	if c.socket == nil {
		return nil
	}
	err := c.socket.Close()
	c.socket = nil
	return err
}
