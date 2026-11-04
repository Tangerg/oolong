package graphics_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	pngcodec "image/png"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/graphics"
)

func TestGraphicsReplyIdentityIsParsedWithoutGuessing(t *testing.T) {
	for _, test := range []struct {
		body                  string
		id, number, placement uint32
		want                  error
	}{
		{"Gi=31;OK", 31, 0, 0, nil},
		{"Gi=99,I=13;OK", 99, 13, 0, nil},
		{"Gi=31,p=4;ENOENT:unknown image", 31, 0, 4, graphics.ErrRejected},
		{"Gi=0,I=13;EINVAL:invalid PNG", 0, 13, 0, graphics.ErrRejected},
		{"G,I=13;ENOMEM:Out of memory", 0, 13, 0, graphics.ErrRejected},
		{"GI=13;ENOMEM:Out of memory", 0, 13, 0, graphics.ErrRejected},
		{"GI=13,i=99;OK", 99, 13, 0, nil},
	} {
		reply, err := graphics.ParseReply(test.body)
		if err != nil || reply.ID != test.id || reply.Number != test.number || reply.Placement != test.placement || !errors.Is(reply.Err(), test.want) {
			t.Fatalf("%q = %+v, %v", test.body, reply, err)
		}
	}
	for _, body := range []string{
		"", "G;OK", "Gi=0;OK", "Gi=-1;OK", "Gi=4294967296;OK", "Gi=1,I=2,I=3;OK", "Gi=1,x=3;OK", "Gi=1;",
		"G,I=13;OK", "GI=13;OK", "G,I=0;ENOMEM", "Gi=0;ENOMEM", "Gi=1,i=1;OK", "G,,I=13;ENOMEM",
	} {
		if _, err := graphics.ParseReply(body); !errors.Is(err, graphics.ErrMalformedReply) {
			t.Fatalf("malformed reply %q = %v", body, err)
		}
	}
}

func TestPNGProbeUsesTheUploadDecoderWithoutRetainingAnImage(t *testing.T) {
	var query bytes.Buffer
	if err := graphics.QueryPNG(&query, 31); err != nil {
		t.Fatal(err)
	}
	payload, found := strings.CutPrefix(query.String(), "\x1b_Gi=31,a=q,t=d,f=100;")
	payload, terminated := strings.CutSuffix(payload, "\x1b\\")
	if !found || !terminated {
		t.Fatalf("query = %q", query.String())
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	image, err := pngcodec.Decode(bytes.NewReader(data))
	if err != nil || image.Bounds().Dx() != 1 || image.Bounds().Dy() != 1 {
		t.Fatalf("probe PNG = %v, %v", image, err)
	}
}

func TestConflictingReplyNumbersCannotChooseARequest(t *testing.T) {
	for _, body := range []string{
		"Gi=12,I=1,I=2;OK", "Gi=12,I=1,I=1;OK", "Gi=12,I=bad,I=2;OK", "Gi=12,I=2,I=bad;OK",
		"G,I=1,I=2;ENOMEM", "G,I=1,I=1;ENOMEM", "G,I=bad,I=2;ENOMEM", "GI=2,I=4294967296;ENOMEM",
	} {
		reply, err := graphics.ParseReply(body)
		if !errors.Is(err, graphics.ErrMalformedReply) || reply.Number != 0 {
			t.Fatalf("%q chose request %d: %v", body, reply.Number, err)
		}
	}
}

func TestMalformedImageIdentityPreservesOnlyUnambiguousRequestNumber(t *testing.T) {
	for _, body := range []string{
		"Gi=broken,I=13;OK", "Gi=1,i=2,I=13;OK", "G,I=13,x=1;ENOMEM", "G,I=13;OK",
	} {
		reply, err := graphics.ParseReply(body)
		if !errors.Is(err, graphics.ErrMalformedReply) || reply.Number != 13 {
			t.Fatalf("%q lost the request identity: %+v, %v", body, reply, err)
		}
	}
}

func TestGraphicsReplyPrefixDistinguishesFramingFromTyping(t *testing.T) {
	for _, test := range []struct {
		body             string
		matched, pending bool
	}{
		{"", false, true},
		{"G", false, true},
		{"Gi", false, true},
		{"GI", false, true},
		{"G,", false, true},
		{"G,I", false, true},
		{"Gi=", true, false},
		{"GI=", true, false},
		{"G,I=13;ENOMEM", true, false},
		{"Gi=bad;OK", true, false},
		{"hello", false, false},
		{"Gx", false, false},
		{"G,Ix", false, false},
		{"G,=", false, false},
	} {
		matched, pending := graphics.MatchReplyPrefix([]byte(test.body))
		if matched != test.matched || pending != test.pending {
			t.Errorf("%q: matched=%t pending=%t", test.body, matched, pending)
		}
	}
}
