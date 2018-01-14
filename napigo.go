// Package napigo interacts with Napiprojekt subtitles API.
package napigo

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

var (
	// ErrSubsNotFound is an error returned when subtitles were not found.
	ErrSubsNotFound = errors.New("subtitles not found")
)

var (
	// URLs used in library.
	searchURL   = "http://napiprojekt.pl/unit_napisy/dl.php"
	downloadURL = "http://napiprojekt.pl/api/api-napiprojekt3.php"
)

const (
	// hashReadSize is amount of data read from the file to compute the hash.
	hashReadSize = 10485760
)

// values used to compute Napi hash.
var (
	idx = []uint16{0xe, 0x3, 0x6, 0x8, 0x2}
	mul = []uint16{2, 2, 5, 4, 3}
	add = []uint16{0, 0xd, 0x10, 0xb, 0x5}
)

// bitmasks used to compute Napi hash.
const (
	lsbMask byte = 0x0f
	msbMask byte = 0xf0
)

// SearchResult is the the result returned by Search function.
type SearchResult struct {
	Lang      string
	Subtitles string
}

// Napi searches and downloads subtitles from Napiprojekt.pl.
type Napi struct {
	client *http.Client
}

// New returnes new Napi.
func New() *Napi {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 180 * time.Second,
	}
	return &Napi{
		client: c,
	}
}

// Hash returnes MD5 hash computed from the first part of video file.
// This hash is used by Napiprojekt to find matching subtitles.
func Hash(fname string) ([]byte, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, hashReadSize)
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}
	h := md5.Sum(buf)
	return h[:], nil
}

// NapiHash returns value computed from video file hash. This value is used to search for subtitles.
func NapiHash(h []byte) string {
	if len(h) != md5.Size {
		return ""
	}
	ret := make([]byte, 0, len(idx))
	for i, iv := range idx {
		a := add[i]
		m := mul[i]

		t := a + val4(h, iv)
		v := val8(h, t)
		x := make([]byte, 2)
		binary.BigEndian.PutUint16(x, v*m)
		ret = append(ret, x[1]&lsbMask)
	}
	return prepRet(ret)
}

func val4(h []byte, idx uint16) uint16 {
	var v byte
	if (idx % 2) == 0 {
		v = (h[idx/2] & msbMask) >> 4
	} else {
		v = h[idx/2] & lsbMask
	}
	return binary.BigEndian.Uint16([]byte{0, v})
}

func val8(h []byte, idx uint16) uint16 {
	if (idx % 2) == 0 {
		return binary.BigEndian.Uint16([]byte{0, h[idx/2]})
	} else {
		v := (h[idx/2] & lsbMask) << 4
		v |= (h[idx/2+1] & msbMask) >> 4
		return binary.BigEndian.Uint16([]byte{0, v})
	}
}

func prepRet(r []byte) string {
	ret := make([]byte, (len(r)+1)/2)
	j := (len(r)+1)/2 - 1
	var x byte
	var save bool
	for i := len(r) - 1; i >= 0; i-- {
		v := r[i]
		if !save {
			x = (v & lsbMask)
			save = true
		} else {
			x |= (v & lsbMask) << 4
			ret[j] = x
			save = false
			j--
		}
	}
	return fmt.Sprintf("%x", ret)[1:]
}

// Search returns list of subtitles found for provided video file and languages.
func (n *Napi) Search(fname string, langs []string, download bool) ([]SearchResult, error) {
	h, err := Hash(fname)
	if err != nil {
		return nil, err
	}
	t := NapiHash(h)
	strHash := fmt.Sprintf("%x", h)
	values := url.Values{}
	values.Add("f", strHash)
	values.Add("t", t)
	values.Add("v", "other")
	values.Add("kolejka", "false")
	values.Add("nick", "")
	values.Add("pass", "")
	values.Add("napios", runtime.GOOS)
	var results []SearchResult
	for _, l := range langs {
		values.Set("l", l)
		data, err := n.doQuery(searchURL + "?" + values.Encode())
		if err != nil {
			return nil, err
		}
		r := SearchResult{
			Lang: l,
		}
		if string(data) == "NPc0" {
			results = append(results, r)
			continue
		}
		if download {
			subs, err := n.download(strHash, l)
			if err != nil {
				return nil, err
			}
			r.Subtitles = subs
			results = append(results, r)
		}
	}

	return results, nil
}

// Download returns string encoded subtitles for provided video file and language.
// If subtitles in provided language don't exists on the Napiprojekt server, Polish subtitles are returned.
// (it's a Napiprojekt behavior).
func (n *Napi) Download(fname, lang string) (string, error) {
	h, err := Hash(fname)
	if err != nil {
		return "", err
	}
	return n.download(fmt.Sprintf("%x", h), lang)
}

func (n *Napi) doQuery(url string) ([]byte, error) {
	resp, err := n.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func (n *Napi) download(hash, lang string) (string, error) {
	v := url.Values{}
	v.Set("downloaded_subtitles_lang", lang)
	v.Set("downloaded_subtitles_txt", "1")
	v.Set("client_ver", "0.1")
	v.Set("downloaded_subtitles_id", hash)
	v.Set("client", "NapiProjektPython") // it has to be set to this value, otherwise we get permission denied.
	v.Set("mode", "1")
	resp, err := n.client.PostForm(downloadURL, v)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	d := xml.NewDecoder(resp.Body)
	r := &result{}
	if err := d.Decode(r); err != nil {
		return "", err
	}
	if r.Status != "success" {
		return "", ErrSubsNotFound
	}
	data, err := base64.StdEncoding.DecodeString(r.Subtitles.Contents)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type result struct {
	Status    string    `xml:"status"`
	Subtitles subtitles `xml:"subtitles"`
}

type subtitles struct {
	Id       string `xml:"id"`
	Contents string `xml:"content"`
}

// SubFileName is a helper function which returns name for the subtitles file.
func SubFileName(fname string) (string, error) {
	els := strings.Split(fname, ".")
	l := len(els)
	if l == 1 {
		return "", fmt.Errorf("incorrect file name %q, no extension", fname)
	}
	return fmt.Sprintf("%s.txt", strings.Join(els[:l-1], ".")), nil
}
