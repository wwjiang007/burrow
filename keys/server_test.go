package keys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/burrow/crypto"
	"github.com/hyperledger/burrow/execution/evm/sha3"
	"github.com/hyperledger/burrow/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type hashInfo struct {
	data     string
	expected string
}

var hashData = map[string]hashInfo{
	"sha256":    {"hi", "8F434346648F6B96DF89DDA901C5176B10A6D83961DD3C1AC88B59B2DC327AA4"},
	"ripemd160": {"hi", "242485AB6BFD3502BCB3442EA2E211687B8E4D89"},
}

var (
	KEY_TYPES  = []string{"ed25519", "secp256k1"}
	HASH_TYPES = []string{"sha256", "ripemd160"}
)

// start the server
func init() {
	failedCh := make(chan error)
	testDir := "test_scratch/" + DefaultKeysDir
	os.RemoveAll(testDir)
	go func() {
		err := StartStandAloneServer(testDir, DefaultHost, TestPort, false, logging.NewNoopLogger())
		failedCh <- err
	}()
	tick := time.NewTicker(time.Second)
	select {
	case err := <-failedCh:
		fmt.Println(err)
		os.Exit(1)
	case <-tick.C:
	}
}

func grpcKeysClient() KeysClient {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial(DefaultHost+":"+TestPort, opts...)
	if err != nil {
		fmt.Printf("Failed to connect to grpc server: %v\n", err)
		os.Exit(1)
	}
	return NewKeysClient(conn)
}

func testServerKeygenAndPub(t *testing.T, typ string) {
	c := grpcKeysClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	genresp, err := c.GenerateKey(ctx, &GenRequest{CurveType: typ})
	if err != nil {
		t.Fatal(err)
	}
	addr := genresp.Address
	resp, err := c.PublicKey(ctx, &PubRequest{Address: addr})
	if err != nil {
		t.Fatal(err)
	}
	addrB, err := crypto.AddressFromHexString(addr)
	if err != nil {
		t.Fatal(err)
	}
	curveType, err := crypto.CurveTypeFromString(typ)
	require.NoError(t, err)
	publicKey, err := crypto.PublicKeyFromBytes(resp.GetPublicKey(), curveType)
	require.NoError(t, err)
	assert.Equal(t, addrB, publicKey.Address())
}

func TestServerKeygenAndPub(t *testing.T) {
	for _, typ := range KEY_TYPES {
		testServerKeygenAndPub(t, typ)
	}
}

func testServerSignAndVerify(t *testing.T, typ string) {
	c := grpcKeysClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	genresp, err := c.GenerateKey(ctx, &GenRequest{CurveType: typ})
	if err != nil {
		t.Fatal(err)
	}
	addr := genresp.Address
	resp, err := c.PublicKey(ctx, &PubRequest{Address: addr})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha3.Sha3([]byte("the hash of something!"))

	sig, err := c.Sign(ctx, &SignRequest{Address: addr, Message: hash})
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Verify(ctx, &VerifyRequest{
		Signature: sig.GetSignature(),
		PublicKey: resp.GetPublicKey(),
		Message:   hash,
		CurveType: typ})
	if err != nil {
		t.Fatal(err)
	}
}

func TestServerSignAndVerify(t *testing.T) {
	for _, typ := range KEY_TYPES {
		testServerSignAndVerify(t, typ)
	}
}

func testServerHash(t *testing.T, typ string) {
	hData := hashData[typ]
	data, expected := hData.data, hData.expected

	c := grpcKeysClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := c.Hash(ctx, &HashRequest{Hashtype: typ, Message: []byte(data)})
	if err != nil {
		t.Fatal(err)
	}
	hash := resp.GetHash()

	if hash != expected {
		t.Fatalf("Hash error for %s. Got %s, expected %s", typ, hash, expected)
	}
}

func TestServerHash(t *testing.T) {
	for _, typ := range HASH_TYPES {
		testServerHash(t, typ)
	}
}

//---------------------------------------------------------------------------------

func checkErrs(t *testing.T, errS string, err error) {
	if err != nil {
		t.Fatal(err)
	}
	if errS != "" {
		t.Fatal(errS)
	}
}
