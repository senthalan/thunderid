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

package pki

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/senthalan/thunder/backend/internal/system/config"
	"github.com/senthalan/thunder/backend/internal/system/error/serviceerror"
	"github.com/senthalan/thunder/backend/internal/system/log"
)

type PKIServiceTestSuite struct {
	suite.Suite
	testPrivateKeyRSA   *rsa.PrivateKey
	testPrivateKeyECDSA *ecdsa.PrivateKey
	testKeyPathRSA      string
	testCertPathRSA     string
	testKeyPathECDSA    string
	testCertPathECDSA   string
	tempFiles           []string
}

func TestPKIServiceSuite(t *testing.T) {
	suite.Run(t, new(PKIServiceTestSuite))
}

func (suite *PKIServiceTestSuite) SetupSuite() {
	// Generate a test RSA private key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(suite.T(), err)
	suite.testPrivateKeyRSA = rsaKey

	// Create a self-signed certificate with RSA key
	certRSA, certPEMRSA, err := createSelfSignedCert(rsaKey)
	assert.NoError(suite.T(), err)

	// Create temporary certificate file for RSA
	certFileRSA, err := os.CreateTemp("", "test_cert_rsa_*.pem")
	assert.NoError(suite.T(), err)
	suite.testCertPathRSA = certFileRSA.Name()
	suite.tempFiles = append(suite.tempFiles, suite.testCertPathRSA)

	_, err = certFileRSA.Write(certPEMRSA)
	assert.NoError(suite.T(), err)
	err = certFileRSA.Close()
	assert.NoError(suite.T(), err)

	// Create temporary private key file for RSA
	keyFileRSA, err := os.CreateTemp("", "test_key_rsa_*.pem")
	assert.NoError(suite.T(), err)
	suite.testKeyPathRSA = keyFileRSA.Name()
	suite.tempFiles = append(suite.tempFiles, suite.testKeyPathRSA)

	privateKeyBytesRSA := x509.MarshalPKCS1PrivateKey(rsaKey)
	privateKeyPEMRSA := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytesRSA,
	})

	_, err = keyFileRSA.Write(privateKeyPEMRSA)
	assert.NoError(suite.T(), err)
	err = keyFileRSA.Close()
	assert.NoError(suite.T(), err)

	// Generate a test ECDSA private key (P-256)
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(suite.T(), err)
	suite.testPrivateKeyECDSA = ecdsaKey

	// Create a self-signed certificate with ECDSA key
	certECDSA, certPEMECDSA, err := createSelfSignedCertEC(ecdsaKey)
	assert.NoError(suite.T(), err)

	// Create temporary certificate file for ECDSA
	certFileECDSA, err := os.CreateTemp("", "test_cert_ecdsa_*.pem")
	assert.NoError(suite.T(), err)
	suite.testCertPathECDSA = certFileECDSA.Name()
	suite.tempFiles = append(suite.tempFiles, suite.testCertPathECDSA)

	_, err = certFileECDSA.Write(certPEMECDSA)
	assert.NoError(suite.T(), err)
	err = certFileECDSA.Close()
	assert.NoError(suite.T(), err)

	// Create temporary private key file for ECDSA
	keyFileECDSA, err := os.CreateTemp("", "test_key_ecdsa_*.pem")
	assert.NoError(suite.T(), err)
	suite.testKeyPathECDSA = keyFileECDSA.Name()
	suite.tempFiles = append(suite.tempFiles, suite.testKeyPathECDSA)

	privateKeyBytesECDSA, err := x509.MarshalPKCS8PrivateKey(ecdsaKey)
	assert.NoError(suite.T(), err)
	privateKeyPEMECDSA := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytesECDSA,
	})

	_, err = keyFileECDSA.Write(privateKeyPEMECDSA)
	assert.NoError(suite.T(), err)
	err = keyFileECDSA.Close()
	assert.NoError(suite.T(), err)

	_ = certRSA   // silence unused
	_ = certECDSA // silence unused
}

func (suite *PKIServiceTestSuite) TearDownSuite() {
	for _, file := range suite.tempFiles {
		err := os.Remove(file)
		if err != nil {
			suite.T().Logf("Failed to remove temp file %s: %v", file, err)
		}
	}
}

func (suite *PKIServiceTestSuite) SetupTest() {
	config.ResetServerRuntime()
}

func (suite *PKIServiceTestSuite) TestGetX509Certificate_Success() {
	// Build a pkiService with an in-memory certificate
	cert, _, err := createSelfSignedCert(suite.testPrivateKeyRSA)
	assert.NoError(suite.T(), err)

	tlsCert := tls.Certificate{Certificate: [][]byte{cert.Raw}}
	pkiSvc := &pkiService{certificates: map[string]PKI{
		"id-1": {
			Certificate: tlsCert,
		},
	}}

	parsed, svcErr := pkiSvc.GetX509Certificate("id-1")
	assert.Nil(suite.T(), svcErr)
	assert.Equal(suite.T(), cert.Raw, parsed.Raw)
}

func (suite *PKIServiceTestSuite) TestGetX509Certificate_NoData() {
	pkiSvc := &pkiService{
		logger: log.GetLogger().With(log.String(log.LoggerKeyComponentName, "PKIService")),
		certificates: map[string]PKI{
			"id-1": {
				Certificate: tls.Certificate{Certificate: [][]byte{}},
			},
		},
	}

	parsed, svcErr := pkiSvc.GetX509Certificate("id-1")
	assert.NotNil(suite.T(), svcErr)
	assert.Nil(suite.T(), parsed)
	assert.Equal(suite.T(), &serviceerror.InternalServerError, svcErr)
}

func (suite *PKIServiceTestSuite) TestGetX509Certificate_ParseError() {
	pkiSvc := &pkiService{
		logger: log.GetLogger().With(log.String(log.LoggerKeyComponentName, "PKIService")),
		certificates: map[string]PKI{
			"id-1": {
				Certificate: tls.Certificate{Certificate: [][]byte{[]byte("bad-cert-bytes")}},
			},
		},
	}

	parsed, svcErr := pkiSvc.GetX509Certificate("id-1")
	assert.NotNil(suite.T(), svcErr)
	assert.Nil(suite.T(), parsed)
	assert.Equal(suite.T(), &serviceerror.InternalServerError, svcErr)
}

func (suite *PKIServiceTestSuite) TestGetX509Certificate_NotFound() {
	pkiSvc := &pkiService{
		logger:       log.GetLogger().With(log.String(log.LoggerKeyComponentName, "PKIService")),
		certificates: map[string]PKI{},
	}
	parsed, svcErr := pkiSvc.GetX509Certificate("missing")
	assert.NotNil(suite.T(), svcErr)
	assert.Equal(suite.T(), &serviceerror.InternalServerError, svcErr)
	assert.Nil(suite.T(), parsed)
}

func (suite *PKIServiceTestSuite) TestInitialize_Success() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: suite.testCertPathRSA,
					KeyFile:  suite.testKeyPathRSA,
				},
				{
					ID:       "test-key-2",
					CertFile: suite.testCertPathECDSA,
					KeyFile:  suite.testKeyPathECDSA,
				},
			},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), pkiService)
	assert.Implements(suite.T(), (*PKIServiceInterface)(nil), pkiService)
}

func (suite *PKIServiceTestSuite) TestInitialize_NoKeyConfigs() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), pkiService)
	assert.Contains(suite.T(), err.Error(), "no key configurations found")
}

func (suite *PKIServiceTestSuite) TestInitialize_MissingCertFile() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: "non_existent_cert.pem",
					KeyFile:  suite.testKeyPathRSA,
				},
			},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), pkiService)
	assert.Contains(suite.T(), err.Error(), "certificate file not found")
}

func (suite *PKIServiceTestSuite) TestInitialize_MissingKeyFile() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: suite.testCertPathRSA,
					KeyFile:  "non_existent_key.pem",
				},
			},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), pkiService)
	assert.Contains(suite.T(), err.Error(), "key file not found")
}

func (suite *PKIServiceTestSuite) TestGetPrivateKey_Success() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: suite.testCertPathRSA,
					KeyFile:  suite.testKeyPathRSA,
				},
			},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.NoError(suite.T(), err)

	privateKey, svcErr := pkiService.GetPrivateKey("test-key")
	assert.Nil(suite.T(), svcErr)
	assert.NotNil(suite.T(), privateKey)
	assert.IsType(suite.T(), &rsa.PrivateKey{}, privateKey)
}

func (suite *PKIServiceTestSuite) TestGetPrivateKey_NotFound() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: suite.testCertPathRSA,
					KeyFile:  suite.testKeyPathRSA,
				},
			},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.NoError(suite.T(), err)

	privateKey, svcErr := pkiService.GetPrivateKey("non-existent-key")
	assert.NotNil(suite.T(), svcErr)
	assert.Equal(suite.T(), &serviceerror.InternalServerError, svcErr)
	assert.Nil(suite.T(), privateKey)
}

func (suite *PKIServiceTestSuite) TestGetCertThumbprint_Success() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: suite.testCertPathRSA,
					KeyFile:  suite.testKeyPathRSA,
				},
			},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.NoError(suite.T(), err)

	thumbprint := pkiService.GetCertThumbprint("test-key")
	assert.NotEmpty(suite.T(), thumbprint)
}

func (suite *PKIServiceTestSuite) TestGetCertThumbprint_NotFound() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: suite.testCertPathRSA,
					KeyFile:  suite.testKeyPathRSA,
				},
			},
		},
	}
	err := config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.NoError(suite.T(), err)

	thumbprint := pkiService.GetCertThumbprint("non-existent-key")
	assert.Empty(suite.T(), thumbprint)
}

func (suite *PKIServiceTestSuite) TestInitialize_WithECDSAKey() {
	// Generate ECDSA P-256 key
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(suite.T(), err)

	// Create self-signed certificate with ECDSA key
	_, certPEM, err := createSelfSignedCertEC(ecKey)
	assert.NoError(suite.T(), err)

	// Write certificate to temp file
	certFile, err := os.CreateTemp("", "test_ec_cert_*.pem")
	assert.NoError(suite.T(), err)
	defer func() {
		err := os.Remove(certFile.Name())
		if err != nil {
			suite.T().Logf("Failed to remove cert file %s: %v", certFile.Name(), err)
		}
	}()

	_, err = certFile.Write(certPEM)
	assert.NoError(suite.T(), err)
	err = certFile.Close()
	assert.NoError(suite.T(), err)

	// Write EC private key to temp file
	ecBytes, err := x509.MarshalECPrivateKey(ecKey)
	assert.NoError(suite.T(), err)

	ecPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: ecBytes,
	})

	keyFile, err := os.CreateTemp("", "test_ec_key_*.pem")
	assert.NoError(suite.T(), err)
	defer func() {
		err := os.Remove(keyFile.Name())
		if err != nil {
			suite.T().Logf("Failed to remove key file %s: %v", keyFile.Name(), err)
		}
	}()

	_, err = keyFile.Write(ecPEM)
	assert.NoError(suite.T(), err)
	err = keyFile.Close()
	assert.NoError(suite.T(), err)

	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "ec-key",
					CertFile: certFile.Name(),
					KeyFile:  keyFile.Name(),
				},
			},
		},
	}
	err = config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), pkiService)

	privKey, svcErr := pkiService.GetPrivateKey("ec-key")
	assert.Nil(suite.T(), svcErr)
	assert.IsType(suite.T(), &ecdsa.PrivateKey{}, privKey)
}

func (suite *PKIServiceTestSuite) TestInitialize_SkipsEmptyIDButLoadsOthers() {
	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{ID: "", CertFile: suite.testCertPathRSA, KeyFile: suite.testKeyPathRSA},
				{ID: "valid", CertFile: suite.testCertPathECDSA, KeyFile: suite.testKeyPathECDSA},
			},
		},
	}
	assert.NoError(suite.T(), config.InitializeServerRuntime("", testConfig))

	pkiService, err := Initialize()
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), pkiService)
	assert.Contains(suite.T(), err.Error(), "empty ID")
}

func (suite *PKIServiceTestSuite) TestInitialize_InvalidCertContent() {
	// Create invalid cert file
	invalidCertFile, err := os.CreateTemp("", "invalid_cert_*.pem")
	assert.NoError(suite.T(), err)
	defer func() { _ = os.Remove(invalidCertFile.Name()) }()
	_, err = invalidCertFile.WriteString("invalid-pem-content")
	assert.NoError(suite.T(), err)
	_ = invalidCertFile.Close()

	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "test-key",
					CertFile: invalidCertFile.Name(), // Invalid file
					KeyFile:  suite.testKeyPathRSA,   // Valid file
				},
			},
		},
	}
	err = config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), pkiService)
	// tls.LoadX509KeyPair returns "failed to find any PEM data" or similar
	assert.Contains(suite.T(), err.Error(), "failed to find any PEM data")
}

func (suite *PKIServiceTestSuite) TestInitialize_UnsupportedCurve() {
	// Generate ECDSA P-224 key (unsupported)
	key, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	assert.NoError(suite.T(), err)

	// Create self-signed cert
	_, certPEM, err := createSelfSignedCertEC(key)
	assert.NoError(suite.T(), err)

	certFile, err := os.CreateTemp("", "test_p224_cert_*.pem")
	assert.NoError(suite.T(), err)
	defer func() { _ = os.Remove(certFile.Name()) }()
	_, err = certFile.Write(certPEM)
	assert.NoError(suite.T(), err)
	_ = certFile.Close()

	keyBytes, err := x509.MarshalECPrivateKey(key)
	assert.NoError(suite.T(), err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	keyFile, err := os.CreateTemp("", "test_p224_key_*.pem")
	assert.NoError(suite.T(), err)
	defer func() { _ = os.Remove(keyFile.Name()) }()
	_, err = keyFile.Write(keyPEM)
	assert.NoError(suite.T(), err)
	_ = keyFile.Close()

	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "p224-key",
					CertFile: certFile.Name(),
					KeyFile:  keyFile.Name(),
				},
			},
		},
	}
	err = config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), pkiService)
	assert.Contains(suite.T(), err.Error(), "unsupported ECDSA curve")
}

func (suite *PKIServiceTestSuite) TestGetThumbprint_Error() {
	badCert := tls.Certificate{Certificate: [][]byte{[]byte("not-a-certificate")}}
	thumbprint, err := getThumbprint(badCert)
	assert.Error(suite.T(), err)
	assert.Empty(suite.T(), thumbprint)
}

func (suite *PKIServiceTestSuite) TestInitialize_WithEd25519Key() {
	// Generate Ed25519 key
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(suite.T(), err)

	// Create self-signed certificate with Ed25519 key
	_, certPEM, err := createSelfSignedCertEd25519(privKey)
	assert.NoError(suite.T(), err)

	// Write certificate to temp file
	certFile, err := os.CreateTemp("", "test_ed_cert_*.pem")
	assert.NoError(suite.T(), err)
	defer func() {
		err := os.Remove(certFile.Name())
		if err != nil {
			suite.T().Logf("Failed to remove cert file %s: %v", certFile.Name(), err)
		}
	}()

	_, err = certFile.Write(certPEM)
	assert.NoError(suite.T(), err)
	err = certFile.Close()
	assert.NoError(suite.T(), err)

	// Write Ed25519 private key to temp file
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	assert.NoError(suite.T(), err)

	edPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	keyFile, err := os.CreateTemp("", "test_ed_key_*.pem")
	assert.NoError(suite.T(), err)
	defer func() {
		err := os.Remove(keyFile.Name())
		if err != nil {
			suite.T().Logf("Failed to remove key file %s: %v", keyFile.Name(), err)
		}
	}()

	_, err = keyFile.Write(edPEM)
	assert.NoError(suite.T(), err)
	err = keyFile.Close()
	assert.NoError(suite.T(), err)

	testConfig := &config.Config{
		Crypto: config.CryptoConfig{
			Keys: []config.KeyConfig{
				{
					ID:       "ed-key",
					CertFile: certFile.Name(),
					KeyFile:  keyFile.Name(),
				},
			},
		},
	}
	err = config.InitializeServerRuntime("", testConfig)
	assert.NoError(suite.T(), err)

	pkiService, err := Initialize()
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), pkiService)

	privKeyRetrieved, svcErr := pkiService.GetPrivateKey("ed-key")
	assert.Nil(suite.T(), svcErr)
	assert.IsType(suite.T(), ed25519.PrivateKey{}, privKeyRetrieved)

	_ = pubKey // silence unused
}

func (suite *PKIServiceTestSuite) TestGetAllX509Certificates_Success() {
	// Create multiple test certificates
	cert1, _, err := createSelfSignedCert(suite.testPrivateKeyRSA)
	assert.NoError(suite.T(), err)

	cert2, _, err := createSelfSignedCertEC(suite.testPrivateKeyECDSA)
	assert.NoError(suite.T(), err)

	// Build a pkiService with multiple in-memory certificates
	tlsCert1 := tls.Certificate{Certificate: [][]byte{cert1.Raw}}
	tlsCert2 := tls.Certificate{Certificate: [][]byte{cert2.Raw}}
	pkiSvc := &pkiService{
		logger: log.GetLogger().With(log.String(log.LoggerKeyComponentName, "PKIService")),
		certificates: map[string]PKI{
			"id-1": {
				Certificate: tlsCert1,
			},
			"id-2": {
				Certificate: tlsCert2,
			},
		},
	}

	allCerts, svcErr := pkiSvc.GetAllX509Certificates()
	assert.Nil(suite.T(), svcErr)
	assert.NotNil(suite.T(), allCerts)
	assert.Equal(suite.T(), 2, len(allCerts))
	assert.Equal(suite.T(), cert1.Raw, allCerts["id-1"].Raw)
	assert.Equal(suite.T(), cert2.Raw, allCerts["id-2"].Raw)
}

func (suite *PKIServiceTestSuite) TestGetAllX509Certificates_Empty() {
	pkiSvc := &pkiService{
		logger:       log.GetLogger().With(log.String(log.LoggerKeyComponentName, "PKIService")),
		certificates: map[string]PKI{},
	}

	allCerts, svcErr := pkiSvc.GetAllX509Certificates()
	assert.Nil(suite.T(), svcErr)
	assert.Equal(suite.T(), map[string]*x509.Certificate{}, allCerts)
}

func (suite *PKIServiceTestSuite) TestGetAllX509Certificates_NoData() {
	pkiSvc := &pkiService{
		logger: log.GetLogger().With(log.String(log.LoggerKeyComponentName, "PKIService")),
		certificates: map[string]PKI{
			"id-1": {
				Certificate: tls.Certificate{Certificate: [][]byte{}},
			},
		},
	}

	allCerts, svcErr := pkiSvc.GetAllX509Certificates()
	assert.NotNil(suite.T(), svcErr)
	assert.Nil(suite.T(), allCerts)
	assert.Equal(suite.T(), &serviceerror.InternalServerError, svcErr)
}

func (suite *PKIServiceTestSuite) TestGetAllX509Certificates_ParseError() {
	cert1, _, err := createSelfSignedCert(suite.testPrivateKeyRSA)
	assert.NoError(suite.T(), err)

	tlsCert1 := tls.Certificate{Certificate: [][]byte{cert1.Raw}}
	pkiSvc := &pkiService{
		logger: log.GetLogger().With(log.String(log.LoggerKeyComponentName, "PKIService")),
		certificates: map[string]PKI{
			"id-1": {
				Certificate: tlsCert1,
			},
			"id-2": {
				Certificate: tls.Certificate{Certificate: [][]byte{[]byte("bad-cert-bytes")}},
			},
		},
	}

	allCerts, svcErr := pkiSvc.GetAllX509Certificates()
	assert.NotNil(suite.T(), svcErr)
	assert.Nil(suite.T(), allCerts)
	assert.Equal(suite.T(), &serviceerror.InternalServerError, svcErr)
}

func (suite *PKIServiceTestSuite) TestGetAlgorithmFromKey_VariantsAndErrors() {
	// P-384
	ec384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	alg, err := getAlgorithmFromKey(ec384)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), P384, alg)

	// P-521
	ec521, _ := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	alg, err = getAlgorithmFromKey(ec521)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), P521, alg)

	// Unsupported curve
	ec224, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	alg, err = getAlgorithmFromKey(ec224)
	assert.Error(suite.T(), err)
	assert.Empty(suite.T(), alg)

	// Unsupported type
	alg, err = getAlgorithmFromKey(123)
	assert.Error(suite.T(), err)
	assert.Empty(suite.T(), alg)
}

// Helper function to create a self-signed certificate
func createSelfSignedCert(privateKey *rsa.PrivateKey) (*x509.Certificate, []byte, error) {
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	parsedCert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, nil, err
	}

	return parsedCert, certPEM, nil
}

// Helper function to create a self-signed certificate with ECDSA key
func createSelfSignedCertEC(privateKey *ecdsa.PrivateKey) (*x509.Certificate, []byte, error) {
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	parsedCert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, nil, err
	}

	return parsedCert, certPEM, nil
}

// Helper function to create a self-signed certificate with Ed25519 key
func createSelfSignedCertEd25519(privateKey ed25519.PrivateKey) (*x509.Certificate, []byte, error) {
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, privateKey.Public(), privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	parsedCert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, nil, err
	}

	return parsedCert, certPEM, nil
}
