package dbms

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/nim4/DBShield/dbshield/logger"
	"github.com/nim4/DBShield/dbshield/sql"
)

//DB2 DBMS
type DB2 struct {
	client      net.Conn
	server      net.Conn
	certificate tls.Certificate
	currentDB   string
	username    string
	reader      func(net.Conn) ([]byte, error)
}

//SetCertificate to use if client asks for SSL
func (d *DB2) SetCertificate(crt, key string) (err error) {
	d.certificate, err = tls.LoadX509KeyPair(crt, key)
	return
}

//SetReader function for sockets IO
func (d *DB2) SetReader(f func(net.Conn) ([]byte, error)) {
	d.reader = f
}

//SetSockets for dbms (client and server sockets)
func (d *DB2) SetSockets(c, s net.Conn) {
	d.client = c
	d.server = s
}

//Close sockets
func (d *DB2) Close() {
	d.client.Close()
	d.server.Close()
}

//DefaultPort of the DBMS
func (d *DB2) DefaultPort() uint {
	return 50000
}

//Handler gets incoming requests
func (d *DB2) Handler() (err error) {
	//defer handlePanic()
	success, err := d.handleLogin()
	if err != nil {
		return
	}
	if !success {
		logger.Warning("Login failed")
		return
	}
	for {
		var buf []byte
		//Read client request
		buf, err = d.reader(d.client)
		if err != nil {
			return
		}
		switch buf[0] {
		case 0x51: //Simple query
			context := sql.QueryContext{
				Query:    string(buf[5:]),
				Database: d.currentDB,
				User:     d.username,
				Client:   remoteAddrToIP(d.client.RemoteAddr()),
				Time:     time.Now().Unix(),
			}
			processContext(context)

		case 0x58: //Terminate
			_, err = d.server.Write(buf)
			return
		}

		//Send request to server
		_, err = d.server.Write(buf)
		if err != nil {
			return
		}

		err = readWrite(d.server, d.client, d.reader)
		if err != nil {
			return
		}
	}
}

func (d *DB2) handleLogin() (success bool, err error) {
	//Receive EXCSAT | ACCSEC
	err = readWrite(d.client, d.server, d.reader)
	if err != nil {
		return
	}

	//Receive EXCSATRD | ACCSECRD
	err = readWrite(d.server, d.client, d.reader)
	if err != nil {
		return
	}

	//Receive Auth
	err = readWrite(d.client, d.server, d.reader)
	if err != nil {
		return
	}

	//Receive result
	buf, err := d.reader(d.server)
	if err != nil {
		return
	}
	//Send result
	_, err = d.client.Write(buf)
	if err != nil {
		return
	}

	for {
		dr, n := parseDRDA(buf)
		success = len(dr.ddm) == 2 && dr.ddm[0] == 0x22 && dr.ddm[1] == 0x01
		buf = buf[n:]
		if success || len(buf) == 0 {
			break
		}
	}
	return
}

type drda struct {
	ddm   []byte
	param []byte
}

func parseDRDA(buf []byte) (dr drda, n int) {
	n = int(buf[0])*256 + int(buf[1])
	dr.ddm = buf[8:10]
	return
}