package device

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// QRVersion is the version of the QR payload format.
const QRVersion = 1

// QRPayload is what the pairing QR carries (api.DevicePairingQRPayload).
// Fingerprint is set only with a self-signed certificate (desktop LAN
// mode): the app pins it.
type QRPayload struct {
	V                 int    `json:"v"`
	URL               string `json:"url"`
	Code              string `json:"code"`
	Fingerprint       string `json:"fingerprint,omitempty"`
	EstablishmentName string `json:"establishment_name,omitempty"`
}

// ErrInvalidQR is returned by DecodeQR for text that is not a pairing QR.
var ErrInvalidQR = errors.New("device: invalid pairing qr")

const qrPrefix = "arenile://pair?d="

// EncodeQR returns the QR text: arenile://pair?d=<base64url(JSON)>.
func EncodeQR(p QRPayload) (string, error) {
	if p.V == 0 {
		p.V = QRVersion
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return qrPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeQR parses a QR text produced by EncodeQR (used by the desktop app
// in remote mode and by tests; the mobile app has its own TypeScript
// decoder generated from the same contract).
func DecodeQR(text string) (QRPayload, error) {
	enc, ok := strings.CutPrefix(text, qrPrefix)
	if !ok {
		return QRPayload{}, ErrInvalidQR
	}
	b, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return QRPayload{}, ErrInvalidQR
	}
	var p QRPayload
	if err := json.Unmarshal(b, &p); err != nil || p.V != QRVersion || p.Code == "" {
		return QRPayload{}, ErrInvalidQR
	}
	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return QRPayload{}, ErrInvalidQR
	}
	return p, nil
}
