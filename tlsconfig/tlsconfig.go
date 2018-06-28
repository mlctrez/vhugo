package tlsconfig

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
)

func TlsConfig(hostname string) (config *tls.Config, err error) {
	client := &http.Client{}

	resp, err := client.Get("http://zuul:9999/newcert/" + hostname)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	io.Copy(buf, resp.Body)
	resp.Body.Close()

	var theCert tls.Certificate
	certData := buf.Bytes()
	for {
		p, rest := pem.Decode(certData)
		if p == nil {
			break
		}
		//fmt.Println(p.Type)
		switch p.Type {
		case "CERTIFICATE":
			if _, err = x509.ParseCertificate(p.Bytes); err != nil {
				return
			}
			theCert.Certificate = append(theCert.Certificate, p.Bytes)
		case "RSA PRIVATE KEY":
			var key *rsa.PrivateKey
			if key, err = x509.ParsePKCS1PrivateKey(p.Bytes); err != nil {
				return
			}
			theCert.PrivateKey = key
		}
		certData = rest
	}

	config = &tls.Config{Certificates: []tls.Certificate{theCert}}
	return
}
