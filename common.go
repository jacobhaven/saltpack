// Copyright 2015 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package saltpack

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"io"

	"github.com/ugorji/go/codec"
	"golang.org/x/crypto/poly1305"
)

// encryptionBlockNumber describes which block number we're at in the sequence
// of encrypted blocks. Each encrypted block of course fits into a packet.
type encryptionBlockNumber uint64

func codecHandle() *codec.MsgpackHandle {
	var mh codec.MsgpackHandle
	mh.WriteExt = true
	return &mh
}

func randomFill(b []byte) (err error) {
	l := len(b)
	n, err := rand.Read(b)
	if err != nil {
		return err
	}
	if n != l {
		return ErrInsufficientRandomness
	}
	return nil
}

func (e encryptionBlockNumber) check() error {
	if e >= encryptionBlockNumber(0xffffffffffffffff) {
		return ErrPacketOverflow
	}
	return nil
}

func writeNullTerminatedString(w io.Writer, s string) {
	w.Write([]byte(s))
	w.Write([]byte{0})
}

func assertEndOfStream(stream *msgpackStream) error {
	var i interface{}
	_, err := stream.Read(&i)
	if err == nil {
		err = ErrTrailingGarbage
	}
	return err
}

func attachedSignatureInput(headerHash []byte, block *SignatureBlock) []byte {
	hasher := sha512.New()
	hasher.Write(headerHash)
	binary.Write(hasher, binary.BigEndian, block.seqno)
	hasher.Write(block.PayloadChunk)

	var buf bytes.Buffer
	writeNullTerminatedString(&buf, SaltpackFormatName)
	writeNullTerminatedString(&buf, SignatureAttachedString)
	buf.Write(hasher.Sum(nil))

	return buf.Bytes()
}

func detachedSignatureInput(headerHash []byte, plaintext []byte) []byte {
	hasher := sha512.New()
	hasher.Write(headerHash)
	hasher.Write(plaintext)

	return detachedSignatureInputFromHash(hasher.Sum(nil))
}

func detachedSignatureInputFromHash(plaintextAndHeaderHash []byte) []byte {
	var buf bytes.Buffer
	writeNullTerminatedString(&buf, SaltpackFormatName)
	writeNullTerminatedString(&buf, SignatureDetachedString)
	buf.Write(plaintextAndHeaderHash)

	return buf.Bytes()
}

func hmacSHA512256(key []byte, input []byte) []byte {
	// Equivalent to crypto_auth, but using Go's builtin HMAC. Truncates
	// SHA512, instead of actually calling SHA512/256.
	if len(key) != CryptoAuthKeyBytes {
		panic("Bad crypto_auth key length")
	}
	authenticatorDigest := hmac.New(sha512.New, key)
	authenticatorDigest.Write(input)
	fullMAC := authenticatorDigest.Sum(nil)
	return fullMAC[:CryptoAuthBytes]
}

func computeMACKey(secret BoxSecretKey, public BoxPublicKey, headerHash []byte) []byte {
	nonce := nonceForMACKeyBox(headerHash)
	macKeyBox := secret.Box(public, nonce, make([]byte, CryptoAuthKeyBytes))
	macKey := macKeyBox[poly1305.TagSize : poly1305.TagSize+CryptoAuthKeyBytes]
	return macKey
}

func computePayloadHash(headerHash []byte, nonce *Nonce, payloadCiphertext []byte) []byte {
	payloadDigest := sha512.New()
	payloadDigest.Write(headerHash)
	payloadDigest.Write(nonce[:])
	payloadDigest.Write(payloadCiphertext)
	return payloadDigest.Sum(nil)
}
