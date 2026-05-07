/*
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Package runtime provides the RuntimeCryptoProvider implementation backed by PKI key material.
package runtime

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/senthalan/thunder/backend/internal/system/crypto"
	"github.com/senthalan/thunder/backend/internal/system/crypto/config"
	"github.com/senthalan/thunder/backend/internal/system/crypto/pki"
	"github.com/senthalan/thunder/backend/internal/system/log"
)

// runtimeCryptoService implements RuntimeCryptoProvider backed by PKI key material.
type runtimeCryptoService struct {
	pkiService pki.PKIServiceInterface
	logger     *log.Logger
}

var (
	runtimeInstance *runtimeCryptoService
	runtimeOnce     sync.Once
)

// GetRuntimeCryptoService returns the singleton RuntimeCryptoProvider instance.
func GetRuntimeCryptoService() crypto.RuntimeCryptoProvider {
	runtimeOnce.Do(func() {
		logger := log.GetLogger().With(log.String(log.LoggerKeyComponentName, "RuntimeCryptoService"))
		pkiSvc, err := pki.Initialize()
		if err != nil {
			logger.Warn("PKI service unavailable; RSA operations will fail", log.String("reason", err.Error()))
		}
		runtimeInstance = &runtimeCryptoService{
			pkiService: pkiSvc,
			logger:     logger,
		}
	})
	return runtimeInstance
}

// Encrypt performs key establishment for the algorithm in params. Per-algorithm behavior:
//   - AlgorithmAESGCM: encrypts content via the config encryption service; returns (ciphertext, nil, err).
//   - AlgorithmRSAOAEP256: content is ignored; generates a random CEK, wraps it with the PKI RSA public key
//     for keyRef, and returns (wrappedCEK, CryptoDetails{CEK}, nil).
//     params.RSAOAEP256.ContentEncryptionAlgorithm must be set.
//   - AlgorithmECDHES: content is ignored; performs ECDH key agreement and derives a CEK via Concat KDF,
//     returning (nil, CryptoDetails{EPK, CEK}, nil). params.ECDHES.ContentEncryptionAlgorithm must be set.
//   - AlgorithmECDHESA128KW / AlgorithmECDHESA256KW: content is ignored; derives a KEK via Concat KDF,
//     generates a random CEK and wraps it, returning (wrappedCEK, CryptoDetails{EPK, CEK}, nil).
//     params.ECDHES.ContentEncryptionAlgorithm must be set.
func (s *runtimeCryptoService) Encrypt(
	ctx context.Context, keyRef crypto.KeyRef, params crypto.AlgorithmParams, content []byte,
) ([]byte, *crypto.CryptoDetails, error) {
	switch params.Algorithm {
	case crypto.AlgorithmAESGCM:
		return s.encryptAESGCM(ctx, content)
	case crypto.AlgorithmRSAOAEP256:
		return s.encryptRSAOAEP256(keyRef, params)
	case crypto.AlgorithmECDHES:
		return s.encryptECDHES(keyRef, params)
	case crypto.AlgorithmECDHESA128KW, crypto.AlgorithmECDHESA256KW:
		return s.encryptECDHESKW(keyRef, params)
	default:
		return nil, nil, fmt.Errorf("unsupported algorithm: %s", params.Algorithm)
	}
}

// Decrypt performs key recovery for the algorithm in params. Per-algorithm behavior:
//   - AlgorithmAESGCM: decrypts content via the config encryption service; returns plaintext.
//   - AlgorithmRSAOAEP256: decrypts content (the wrapped CEK) with the PKI RSA private key for keyRef;
//     returns the unwrapped CEK.
//   - AlgorithmECDHES: content is ignored; re-derives the CEK from params.ECDHES.EPK and the private key
//     via Concat KDF, returning the derived CEK. params.ECDHES.EPK and
//     params.ECDHES.ContentEncryptionAlgorithm must be set.
//   - AlgorithmECDHESA128KW / AlgorithmECDHESA256KW: re-derives KEK from params.ECDHES.EPK, then unwraps
//     content (the wrapped CEK) using AES Key Unwrap; returns the plain CEK. params.ECDHES.EPK must be set.
func (s *runtimeCryptoService) Decrypt(
	ctx context.Context, keyRef crypto.KeyRef, params crypto.AlgorithmParams, content []byte,
) ([]byte, error) {
	switch params.Algorithm {
	case crypto.AlgorithmAESGCM:
		return s.decryptAESGCM(ctx, content)
	case crypto.AlgorithmRSAOAEP256:
		return s.decryptRSAOAEP256(keyRef, content)
	case crypto.AlgorithmECDHES:
		return s.decryptECDHES(keyRef, params)
	case crypto.AlgorithmECDHESA128KW, crypto.AlgorithmECDHESA256KW:
		return s.decryptECDHESKW(keyRef, params, content)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", params.Algorithm)
	}
}

func (s *runtimeCryptoService) encryptAESGCM(
	ctx context.Context, content []byte,
) ([]byte, *crypto.CryptoDetails, error) {
	ciphertext, err := config.GetEncryptionService().Encrypt(ctx, content)
	return ciphertext, nil, err
}

func (s *runtimeCryptoService) encryptRSAOAEP256(
	keyRef crypto.KeyRef, params crypto.AlgorithmParams,
) ([]byte, *crypto.CryptoDetails, error) {
	if s.pkiService == nil {
		return nil, nil, errors.New("PKI service not initialized")
	}
	cert, svcErr := s.pkiService.GetX509Certificate(keyRef.KeyID)
	if svcErr != nil {
		return nil, nil, fmt.Errorf("key not found for id %s: [%s] %s",
			keyRef.KeyID, svcErr.Code, svcErr.Error.DefaultValue)
	}
	rsaPub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, nil, errors.New("key is not an RSA public key")
	}
	if params.RSAOAEP256.ContentEncryptionAlgorithm == "" {
		return nil, nil, errors.New("ContentEncryptionAlgorithm required for RSA-OAEP-256 CEK generation")
	}
	cekLen, err := ecdhContentEncKeyLen(params.RSAOAEP256.ContentEncryptionAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	cek := make([]byte, cekLen)
	if _, err := rand.Read(cek); err != nil {
		return nil, nil, fmt.Errorf("CEK generation failed: %w", err)
	}
	encryptedCEK, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPub, cek, nil)
	return encryptedCEK, &crypto.CryptoDetails{CEK: cek}, err
}

func (s *runtimeCryptoService) encryptECDHES(
	keyRef crypto.KeyRef, params crypto.AlgorithmParams,
) ([]byte, *crypto.CryptoDetails, error) {
	ecdsaPub, err := s.getECPublicKey(keyRef)
	if err != nil {
		return nil, nil, err
	}
	ephemeralPriv, ephemeralPub, err := ecdhGenerateEphemeralKeyPair(ecdsaPub)
	if err != nil {
		return nil, nil, fmt.Errorf("ephemeral key generation failed: %w", err)
	}
	z, err := ecdhComputeSharedSecret(ephemeralPriv, ecdsaPub)
	if err != nil {
		return nil, nil, fmt.Errorf("ECDH key agreement failed: %w", err)
	}
	if params.ECDHES.ContentEncryptionAlgorithm == "" {
		return nil, nil, errors.New("ContentEncryptionAlgorithm required for ECDH-ES key derivation")
	}
	keyLen, err := ecdhContentEncKeyLen(params.ECDHES.ContentEncryptionAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	derivedCEK, err := ecdhConcatKDF(z, string(params.ECDHES.ContentEncryptionAlgorithm), keyLen)
	if err != nil {
		return nil, nil, fmt.Errorf("key derivation failed: %w", err)
	}
	return nil, &crypto.CryptoDetails{EPK: ephemeralPub, CEK: derivedCEK}, nil
}

func (s *runtimeCryptoService) encryptECDHESKW(
	keyRef crypto.KeyRef, params crypto.AlgorithmParams,
) ([]byte, *crypto.CryptoDetails, error) {
	ecdsaPub, err := s.getECPublicKey(keyRef)
	if err != nil {
		return nil, nil, err
	}
	ephemeralPriv, ephemeralPub, err := ecdhGenerateEphemeralKeyPair(ecdsaPub)
	if err != nil {
		return nil, nil, fmt.Errorf("ephemeral key generation failed: %w", err)
	}
	z, err := ecdhComputeSharedSecret(ephemeralPriv, ecdsaPub)
	if err != nil {
		return nil, nil, fmt.Errorf("ECDH key agreement failed: %w", err)
	}
	kekLen := 16
	if params.Algorithm == crypto.AlgorithmECDHESA256KW {
		kekLen = 32
	}
	kek, err := ecdhConcatKDF(z, string(params.Algorithm), kekLen)
	if err != nil {
		return nil, nil, fmt.Errorf("key derivation failed: %w", err)
	}
	if params.ECDHES.ContentEncryptionAlgorithm == "" {
		return nil, nil, errors.New("ContentEncryptionAlgorithm required for ECDH-ES+KW CEK generation")
	}
	cekLen, err := ecdhContentEncKeyLen(params.ECDHES.ContentEncryptionAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	cek := make([]byte, cekLen)
	if _, err := rand.Read(cek); err != nil {
		return nil, nil, fmt.Errorf("CEK generation failed: %w", err)
	}
	wrappedKey, err := ecdhAESKeyWrap(kek, cek)
	if err != nil {
		return nil, nil, fmt.Errorf("AES key wrap failed: %w", err)
	}
	return wrappedKey, &crypto.CryptoDetails{EPK: ephemeralPub, CEK: cek}, nil
}

func (s *runtimeCryptoService) decryptAESGCM(ctx context.Context, content []byte) ([]byte, error) {
	return config.GetEncryptionService().Decrypt(ctx, content)
}

func (s *runtimeCryptoService) decryptRSAOAEP256(keyRef crypto.KeyRef, content []byte) ([]byte, error) {
	if s.pkiService == nil {
		return nil, errors.New("PKI service not initialized")
	}
	privKey, svcErr := s.pkiService.GetPrivateKey(keyRef.KeyID)
	if svcErr != nil {
		return nil, fmt.Errorf("key not found for id %s: [%s] %s",
			keyRef.KeyID, svcErr.Code, svcErr.Error.DefaultValue)
	}
	rsaPriv, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an RSA private key")
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, rsaPriv, content, nil)
}

func (s *runtimeCryptoService) decryptECDHES(
	keyRef crypto.KeyRef, params crypto.AlgorithmParams,
) ([]byte, error) {
	ecdsaPriv, epk, err := s.getECDHDecryptKeys(keyRef, params, "ECDH-ES")
	if err != nil {
		return nil, err
	}
	z, err := ecdhComputeSharedSecretForRecipient(ecdsaPriv, epk)
	if err != nil {
		return nil, fmt.Errorf("ECDH key agreement failed: %w", err)
	}
	if params.ECDHES.ContentEncryptionAlgorithm == "" {
		return nil, errors.New("ContentEncryptionAlgorithm required for ECDH-ES key derivation")
	}
	keyLen, err := ecdhContentEncKeyLen(params.ECDHES.ContentEncryptionAlgorithm)
	if err != nil {
		return nil, err
	}
	return ecdhConcatKDF(z, string(params.ECDHES.ContentEncryptionAlgorithm), keyLen)
}

func (s *runtimeCryptoService) decryptECDHESKW(
	keyRef crypto.KeyRef, params crypto.AlgorithmParams, content []byte,
) ([]byte, error) {
	ecdsaPriv, epk, err := s.getECDHDecryptKeys(keyRef, params, "ECDH-ES+KW")
	if err != nil {
		return nil, err
	}
	z, err := ecdhComputeSharedSecretForRecipient(ecdsaPriv, epk)
	if err != nil {
		return nil, fmt.Errorf("ECDH key agreement failed: %w", err)
	}
	kekLen := 16
	if params.Algorithm == crypto.AlgorithmECDHESA256KW {
		kekLen = 32
	}
	kek, err := ecdhConcatKDF(z, string(params.Algorithm), kekLen)
	if err != nil {
		return nil, fmt.Errorf("key derivation failed: %w", err)
	}
	return ecdhAESKeyUnwrap(kek, content)
}

func (s *runtimeCryptoService) getECPublicKey(keyRef crypto.KeyRef) (*ecdsa.PublicKey, error) {
	if s.pkiService == nil {
		return nil, errors.New("PKI service not initialized")
	}
	cert, svcErr := s.pkiService.GetX509Certificate(keyRef.KeyID)
	if svcErr != nil {
		return nil, fmt.Errorf("key not found for id %s: [%s] %s",
			keyRef.KeyID, svcErr.Code, svcErr.Error.DefaultValue)
	}
	ecdsaPub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("key is not an EC public key")
	}
	return ecdsaPub, nil
}

func (s *runtimeCryptoService) getECDHDecryptKeys(
	keyRef crypto.KeyRef, params crypto.AlgorithmParams, algorithm string,
) (*ecdsa.PrivateKey, *ecdh.PublicKey, error) {
	if s.pkiService == nil {
		return nil, nil, errors.New("PKI service not initialized")
	}
	if params.ECDHES.EPK == nil {
		return nil, nil, fmt.Errorf("EPK required for %s decryption", algorithm)
	}
	epk, ok := params.ECDHES.EPK.(*ecdh.PublicKey)
	if !ok {
		return nil, nil, errors.New("EPK must be an *ecdh.PublicKey")
	}
	privKey, svcErr := s.pkiService.GetPrivateKey(keyRef.KeyID)
	if svcErr != nil {
		return nil, nil, fmt.Errorf("key not found for id %s: [%s] %s",
			keyRef.KeyID, svcErr.Code, svcErr.Error.DefaultValue)
	}
	ecdsaPriv, ok := privKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("key is not an EC private key")
	}
	return ecdsaPriv, epk, nil
}

// Sign is not yet implemented.
func (s *runtimeCryptoService) Sign(_ context.Context, _ crypto.KeyRef, _ crypto.Algorithm, _ []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}

// GetPublicKeys is not yet implemented.
func (s *runtimeCryptoService) GetPublicKeys(
	_ context.Context, _ crypto.PublicKeyFilter,
) ([]crypto.PublicKeyInfo, error) {
	return nil, errors.New("not implemented")
}

// GetTLSMaterial is not yet implemented.
func (s *runtimeCryptoService) GetTLSMaterial(_ context.Context, _ crypto.KeyRef) (*crypto.TLSMaterial, error) {
	return nil, errors.New("not implemented")
}
