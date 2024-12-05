package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

var logger *log.Logger
var fileLogger *log.Logger
var hostDir string = "./"
var HTTPPORT int
var HTTPSPORT int
var FTPPORT int
var passivePortStart int = 49152
var passivePortEnd int = 65534

func genCert() {
	if _, err := os.Stat(fmt.Sprintf("%s/cert.pem", hostDir)); err == nil {
		if _, er := os.Stat(fmt.Sprintf("%s/key.pem", hostDir)); er == nil {
			fmt.Println("[*] Found certificate files in directory. Using these.")
			return
		}
	}
	fmt.Println("[*] No certificate files found in directory. Generating new...")
	s, _ := rand.Prime(rand.Reader, 1024)
	ca := &x509.Certificate{
		SerialNumber: s,
		Subject: pkix.Name{
			Country:      []string{"GB"},
			Organization: []string{"4ARMED"},
			CommonName:   "*.4armed.io",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		SubjectKeyId:          []byte{1, 2, 3, 4, 6},
		BasicConstraintsValid: true,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 1024)
	pub := &priv.PublicKey
	ca_b, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)
	if err != nil {
		fmt.Println("create ca failed", err)
	}

	kpemfile, err := os.Create("key.pem")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cpemfile, err := os.Create("cert.pem")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var pemkey = &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv)}
	err = pem.Encode(kpemfile, pemkey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	kpemfile.Close()
	pem.Encode(cpemfile, &pem.Block{Type: "CERTIFICATE", Bytes: ca_b})
	cpemfile.Close()

	fmt.Println("[*] Certificate files generated")
}

func parseConn(conn *net.TCPConn, passive bool) {
	writer := io.Writer(conn)

	if !passive {
		writer.Write([]byte("220 FTP\r\n"))
	}
	var olog *log.Logger

	if fileLogger != nil {
		olog = fileLogger
	} else {
		olog = log.New(os.Stderr, "", 0)
	}

	buf := &bytes.Buffer{}
	reserved := []string{"EPRT"}
	for {
		data := make([]byte, 2048)
		n, err := conn.Read(data)

		if err != nil {
			logger.Println("[x] Connection Closed")
			break
		}

		buf.Write(data[:n])

		if buf.Len() > 4 {
			cmd := string(buf.Bytes()[:4])
			if cmd == "USER" {
				olog.Printf("%s: %s", cmd, string(buf.Bytes()[4:]))
				writer.Write([]byte("331 password please - version check\r\n"))
			} else if cmd == "PASS" {
				olog.Printf("%s: %s", cmd, string(buf.Bytes()[4:]))
				writer.Write([]byte("230 User logged in\r\n"))
			} else if cmd == "QUIT" {
				writer.Write([]byte("221 Goodbye.\r\n"))
				break
			} else if cmd == "RETR" {
				olog.Printf("%s", string(buf.Bytes()[4:]))
				writer.Write([]byte("451 Nope\r\n"))
				writer.Write([]byte("221 Goodbye.\r\n"))
				break
			} else if cmd == "PASV" {
				olog.Printf("Entering Passive Mode")

				// port := getPassivePort()
				// startFTP(port, true)

				port := FTPPORT

				p1 := port / 256
				p2 := port - (p1 * 256)
				ip := strings.Split(conn.LocalAddr().String(), ":")[0]
				quads := strings.Split(ip, ".")
				reply := fmt.Sprintf("227 Entering Passive Mode (%s,%s,%s,%s,%d,%d)\r\n", quads[0], quads[1], quads[2], quads[3], p1, p2)
				writer.Write([]byte(reply))
			} else if cmd == "EPSV" {
				olog.Printf("Entering Extended Passive Mode")

				port := FTPPORT

				// p1 := port / 256
				// p2 := port - (p1 * 256)
				// ip := strings.Split(conn.LocalAddr().String(), ":")[0]
				// quads := strings.Split(ip, ".")
				reply := fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|||)\r\n", port)
				writer.Write([]byte(reply))
			} else if cmd == "STOR" {
				olog.Printf("%s", buf.String())
				writer.Write([]byte("150 Using transfer connection\r\n"))
			} else if cmd == "TYPE" {
				writer.Write([]byte("200 Type set to I\r\n"))
			} else {
				if string(buf.Bytes()[:3]) == "CWD" {
					writer.Write([]byte("250 Directory successfully changed.\r\n"))
					olog.Printf("/%s", strings.Replace(string(buf.Bytes()[4:]), "\r\n", "", 1))
				} else if string(buf.Bytes()[:3]) == "PWD" {
					writer.Write([]byte("257 \"/\" is the current directory\r\n"))
					olog.Printf("/%s", strings.Replace(string(buf.Bytes()[4:]), "\r\n", "", 1))
				} else if contains(reserved, string(buf.Bytes()[:4])) {
					writer.Write([]byte("230 more data please!\r\n"))
				} else {
					writer.Write([]byte("230 more data please!\r\n"))
					olog.Printf("%s\n", string(buf.Bytes()[:4]))
				}
			}
		}
		buf = &bytes.Buffer{}
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func handleConnection(incomming <-chan *net.TCPConn, outgoing chan<- *net.TCPConn, passive bool) {
	for conn := range incomming {
		parseConn(conn, passive)
		outgoing <- conn
	}
}

func closeConnection(incomming <-chan *net.TCPConn) {
	for conn := range incomming {
		logger.Println("[*] Closing FTP Connection")
		conn.Close()
	}
}

func logRequest(w http.ResponseWriter, req *http.Request) {
	if _, err := os.Stat(fmt.Sprintf("%s/%s", hostDir, req.URL.Path)); err != nil {
		logger.Printf("[%s][404] %s\n", req.RemoteAddr, req.URL)
		fmt.Fprintf(w, "Not Found")
	} else {
		logger.Printf("[%s][200] %s\n", req.RemoteAddr, req.URL)
		if req.URL.Path == "/" {
			http.ServeFile(w, req, fmt.Sprintf("%s/", hostDir))
		} else if req.URL.Path[len(req.URL.Path)-1:] == "/" {
			http.ServeFile(w, req, fmt.Sprintf("%s/%s", hostDir, req.URL.Path[:len(req.URL.Path)-1]))
		} else {
			http.ServeFile(w, req, fmt.Sprintf("%s/%s", hostDir, req.URL.Path))
		}
	}
}

func serveWeb(dir string) {
	logger.Printf("[*] Starting Web Server on %d [%s]\n", HTTPPORT, dir)
	hostDir = dir
	http.HandleFunc("/", logRequest)
	go http.ListenAndServe(fmt.Sprint(":", HTTPPORT), nil)
	genCert()
	go http.ListenAndServeTLS(fmt.Sprintf(":%d", HTTPSPORT), "cert.pem", "key.pem", nil)
}

// func acceptConnections(ls *net.TCPListener, waiting chan<- *net.TCPConn) {
// 	for {
// 		conn, err := ls.AcceptTCP()
// 		if err != nil {
// 			logger.Fatal("[x] - Failed to accept connection\n", err)
// 		}
// 		logger.Printf("[*] Connection Accepted from [%s]\n", conn.RemoteAddr().String())
// 		waiting <- conn
// 	}
// }

func startFTP(port int, passive bool) func() error {
	waiting, complete := make(chan *net.TCPConn), make(chan *net.TCPConn)
	var err error

	for i := 0; i < 1; i++ {
		go handleConnection(waiting, complete, passive)
	}
	go closeConnection(complete)

	var clientConn *net.TCPConn

	addr, _ := net.ResolveTCPAddr("tcp", fmt.Sprint(":", port))
	ls, err := net.ListenTCP("tcp", addr)
	if err != nil {
		logger.Fatal("[x] - Failed to start connection\n", err)
	}

	logger.Println("[*] FTP Server - Port: ", port)

	for {
		clientConn, err = ls.AcceptTCP()
		if err != nil {
			logger.Fatal("[x] - Failed to accept connection\n", err)
		}
		logger.Printf("[*] Connection Accepted from [%s]\n", clientConn.RemoteAddr().String())
		waiting <- clientConn
	}

	// acceptConnections(ls, waiting)

	// return func() error {
	// 	if clientConn != nil {
	// 		clientConn.Close()
	// 	}
	// 	return ls.Close()
	// }
}

func getPassivePort() int {
	var port int
	for i := passivePortStart; i < passivePortEnd; i++ {
		port = passivePortStart + mathrand.Intn(passivePortEnd-passivePortStart)
	}

	return port
}

func startUno(port int) {
	conn, err := net.Listen("tcp", fmt.Sprint(":", port))
	if err != nil {
		panic("failed to connect: " + err.Error())
	}

	fmt.Println("[*] UNO Listening...")

	for {
		cl, err := conn.Accept()
		if err != nil {
			fmt.Printf("server: accept: %s", err)
			break
		}
		fmt.Printf("[*] UNO Accepted from: %s\n", cl.RemoteAddr())
		go parseUnoConnection(cl)
	}

}
func passerby(conn1, conn2 net.Conn, reader bufio.Reader, userreader bool, done chan<- bool) {
	var err error
	var n int
	for {
		if userreader {
			data := make([]byte, 256)
			n, err = reader.Read(data)

			//if n > 0 {
			conn1.Write(data[:n])
			//}
		} else {
			_, err = io.Copy(conn1, conn2)
		}
		if err != nil && err == io.EOF {
			break
		}
	}
	done <- true
}
func connectInternal(conn net.Conn, port int, reader bufio.Reader) {
	var err error
	var connR net.Conn

	connR, err = net.Dial("tcp", fmt.Sprint("127.0.0.1:", port))

	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("[*] Connected to Internal server:", connR.RemoteAddr())

	breakC, breakS := make(chan bool, 1), make(chan bool, 1)
	go passerby(connR, conn, reader, true, breakC)
	go passerby(conn, connR, reader, false, breakS)

	select {
	case <-breakS:
	case <-breakC:
		connR.Close()
		conn.Close()
	}
}

func parseUnoConnection(conn net.Conn) {
	timeout := make(chan bool, 1)
	typex := make(chan []byte, 1)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	go func() {
		time.Sleep(3 * time.Second)
		timeout <- true
	}()

	reader := bufio.NewReader(conn)
	go func() {
		status, _ := reader.Peek(1)
		typex <- status
	}()

	select {
	case <-timeout:
		// the read from ch has timed out
		fmt.Println("Timout triggered")
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		connectInternal(conn, FTPPORT, *reader)
	case k := <-typex:
		reader.UnreadByte()
		if len(k) < 1 {
			fmt.Println("unkown connection")
			return
		}
		if k[0] == 22 { //https
			connectInternal(conn, HTTPSPORT, *reader)
		} else if k[0] == 71 {
			connectInternal(conn, HTTPPORT, *reader)
		} else {
			fmt.Println("unkown connection")
		}
	}
}

func main() {
	unoPortPtr := flag.Int("uno", 5000, "Global port to listen on")
	portPtr := flag.Int("p", 2121, "Port to listen on")
	webEnabledPtr := flag.Bool("w", false, "Setup web-server for DTDs")
	webPortPtr := flag.Int("wp", 2122, "Port to serve DTD on")
	webPortSPtr := flag.Int("wps", 2123, "SSL Port to serve DTD on")
	webFolderPtr := flag.String("wd", "./", "Folder to server DTD(s) from")
	fileLog := flag.String("o", "", "File location to log to")
	flag.Parse()

	FTPPORT = *portPtr
	HTTPPORT = *webPortPtr
	HTTPSPORT = *webPortSPtr

	logger = log.New(os.Stderr, "", log.LstdFlags)
	go startUno(*unoPortPtr)
	if *fileLog != "" {
		if _, err := os.Stat(*fileLog); os.IsNotExist(err) {
			logger.Println("[*] File doesn't exist, creating")
			if _, err := os.Create(*fileLog); err != nil {
				logger.Fatal("[x] Unable to create log file! Exiting.")
			}
		}
		errorlog, err := os.OpenFile(*fileLog, os.O_RDWR, 0666)
		if err != nil {
			logger.Fatalf("error opening file: %v", err)
		}
		fileLogger = log.New(errorlog, "", 0)
		logger.Printf("[*] Storing session into the file: %s", *fileLog)
	}

	if *webEnabledPtr {
		serveWeb(*webFolderPtr)
	}
	startFTP(FTPPORT, false)
}
