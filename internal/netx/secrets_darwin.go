//go:build darwin

package netx

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The Keychain item is a generic password with the attributes the Swift app
// writes (ProxyCredentialStore.query): class genp, service, account = the
// credential UUID, synchronizable = false, value = the credentials JSON. It is
// bound with purego instead of go-keyring, whose macOS backend base64-wraps the
// value and could not be read back by the Swift app.

const (
	securityPath = "/System/Library/Frameworks/Security.framework/Security"
	cfPath       = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"

	kcStringEncodingUTF8 = 0x08000100
	errSecSuccess        = 0
	errSecItemNotFound   = -25300
)

type keychainAPI struct {
	stringCreate func(alloc uintptr, cstr string, encoding uint32) uintptr
	dataCreate   func(alloc uintptr, bytes *byte, length int) uintptr
	dataLength   func(data uintptr) int
	dataBytes    func(data uintptr) unsafe.Pointer
	typeID       func(ref uintptr) uintptr
	dataTypeID   func() uintptr
	dictCreate   func(alloc uintptr, keys, values *uintptr, count int, keyCB, valueCB uintptr) uintptr
	release      func(ref uintptr)

	copyMatching func(query uintptr, result *uintptr) int32
	update       func(query, attrs uintptr) int32
	add          func(attrs uintptr, result *uintptr) int32
	remove       func(query uintptr) int32
	getInteract  func(allowed *uint8) int32
	setInteract  func(allowed uint8) int32

	keyCB, valueCB                               uintptr
	kClass, kClassGeneric, kService, kAccount    uintptr
	kSynchronizable, kValueData, kReturnData     uintptr
	kMatchLimit, kMatchLimitOne, cfTrue, cfFalse uintptr
}

var (
	kcOnce sync.Once
	kc     keychainAPI
	kcErr  error

	// Like the Swift store, serialize every Keychain operation: the
	// user-interaction switch below is process-wide.
	kcMu sync.Mutex
)

func loadKeychain() (*keychainAPI, error) {
	kcOnce.Do(func() { kcErr = kc.bind() })
	return &kc, kcErr
}

func (k *keychainAPI) bind() error {
	security, err := purego.Dlopen(securityPath, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return fmt.Errorf("load Security: %w", err)
	}
	cf, err := purego.Dlopen(cfPath, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return fmt.Errorf("load CoreFoundation: %w", err)
	}
	funcs := []struct {
		dst  any
		lib  uintptr
		name string
	}{
		{&k.stringCreate, cf, "CFStringCreateWithCString"},
		{&k.dataCreate, cf, "CFDataCreate"},
		{&k.dataLength, cf, "CFDataGetLength"},
		{&k.dataBytes, cf, "CFDataGetBytePtr"},
		{&k.typeID, cf, "CFGetTypeID"},
		{&k.dataTypeID, cf, "CFDataGetTypeID"},
		{&k.dictCreate, cf, "CFDictionaryCreate"},
		{&k.release, cf, "CFRelease"},
		{&k.copyMatching, security, "SecItemCopyMatching"},
		{&k.update, security, "SecItemUpdate"},
		{&k.add, security, "SecItemAdd"},
		{&k.remove, security, "SecItemDelete"},
		{&k.getInteract, security, "SecKeychainGetUserInteractionAllowed"},
		{&k.setInteract, security, "SecKeychainSetUserInteractionAllowed"},
	}
	for _, f := range funcs {
		addr, err := purego.Dlsym(f.lib, f.name)
		if err != nil {
			return fmt.Errorf("load %s: %w", f.name, err)
		}
		purego.RegisterFunc(f.dst, addr)
	}
	// Exported constants: the symbol is the address of the variable that holds
	// the CFTypeRef (callbacks are structs, whose address is passed as is).
	for _, c := range []struct {
		dst  *uintptr
		lib  uintptr
		name string
	}{
		{&k.kClass, security, "kSecClass"}, {&k.kClassGeneric, security, "kSecClassGenericPassword"},
		{&k.kService, security, "kSecAttrService"}, {&k.kAccount, security, "kSecAttrAccount"},
		{&k.kSynchronizable, security, "kSecAttrSynchronizable"}, {&k.kValueData, security, "kSecValueData"},
		{&k.kReturnData, security, "kSecReturnData"}, {&k.kMatchLimit, security, "kSecMatchLimit"},
		{&k.kMatchLimitOne, security, "kSecMatchLimitOne"},
		{&k.cfTrue, cf, "kCFBooleanTrue"}, {&k.cfFalse, cf, "kCFBooleanFalse"},
	} {
		addr, err := purego.Dlsym(c.lib, c.name)
		if err != nil {
			return fmt.Errorf("load %s: %w", c.name, err)
		}
		*c.dst = **(**uintptr)(unsafe.Pointer(&addr))
	}
	for _, c := range []struct {
		dst  *uintptr
		name string
	}{{&k.keyCB, "kCFTypeDictionaryKeyCallBacks"}, {&k.valueCB, "kCFTypeDictionaryValueCallBacks"}} {
		addr, err := purego.Dlsym(cf, c.name)
		if err != nil {
			return fmt.Errorf("load %s: %w", c.name, err)
		}
		*c.dst = addr
	}
	return nil
}

// dict builds a CFDictionary from key/value CF objects. The caller releases it.
func (k *keychainAPI) dict(pairs ...uintptr) uintptr {
	keys := make([]uintptr, 0, len(pairs)/2)
	values := make([]uintptr, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		keys = append(keys, pairs[i])
		values = append(values, pairs[i+1])
	}
	return k.dictCreate(0, &keys[0], &values[0], len(keys), k.keyCB, k.valueCB)
}

// query builds the identifying attributes shared by every operation, plus any
// extra key/value pairs. It returns the dictionary and the temporary CF
// objects to release.
func (k *keychainAPI) query(service, id string, extra ...uintptr) (uintptr, []uintptr) {
	svc, acct := k.stringCreate(0, service, kcStringEncodingUTF8), k.stringCreate(0, id, kcStringEncodingUTF8)
	pairs := append([]uintptr{
		k.kClass, k.kClassGeneric,
		k.kService, svc,
		k.kAccount, acct,
		k.kSynchronizable, k.cfFalse,
	}, extra...)
	return k.dict(pairs...), []uintptr{svc, acct}
}

type keychainSecrets struct{ service string }

func newPlatformSecrets(service string) rawSecrets { return keychainSecrets{service: service} }

func (s keychainSecrets) get(id string, allowInteraction bool) ([]byte, bool, error) {
	k, err := loadKeychain()
	if err != nil {
		return nil, false, err
	}
	kcMu.Lock()
	defer kcMu.Unlock()
	q, tmp := k.query(s.service, id, k.kReturnData, k.cfTrue, k.kMatchLimit, k.kMatchLimitOne)
	defer k.releaseAll(append(tmp, q))

	// Items live in the legacy (file-based) Keychain, which ignores
	// LAContext. Its process-wide interaction switch is the only way to forbid
	// the access prompt for a background load, as in the Swift store; restore
	// it after.
	var previous uint8
	if !allowInteraction {
		if k.getInteract(&previous) != errSecSuccess || k.setInteract(0) != errSecSuccess {
			return nil, false, fmt.Errorf("keychain interaction switch unavailable")
		}
	}
	var result uintptr
	status := k.copyMatching(q, &result)
	if !allowInteraction && k.setInteract(previous) != errSecSuccess {
		if result != 0 {
			k.release(result)
		}
		return nil, false, fmt.Errorf("keychain interaction switch not restored")
	}
	switch {
	case status == errSecItemNotFound:
		return nil, false, nil
	case status != errSecSuccess || result == 0:
		return nil, false, fmt.Errorf("keychain read failed (OSStatus %d)", status)
	}
	defer k.release(result)
	if k.typeID(result) != k.dataTypeID() {
		return nil, false, fmt.Errorf("keychain item is not data")
	}
	n := k.dataLength(result)
	data := make([]byte, n)
	if n > 0 {
		copy(data, unsafe.Slice((*byte)(k.dataBytes(result)), n))
	}
	return data, true, nil
}

func (s keychainSecrets) set(id string, data []byte) error {
	k, err := loadKeychain()
	if err != nil {
		return err
	}
	kcMu.Lock()
	defer kcMu.Unlock()
	var first byte
	ptr := &first
	if len(data) > 0 {
		ptr = &data[0]
	}
	value := k.dataCreate(0, ptr, len(data))
	defer k.release(value)

	q, tmp := k.query(s.service, id)
	defer k.releaseAll(append(tmp, q))
	values := k.dict(k.kValueData, value)
	defer k.release(values)
	status := k.update(q, values)
	if status == errSecItemNotFound {
		item, tmp2 := k.query(s.service, id, k.kValueData, value)
		defer k.releaseAll(append(tmp2, item))
		status = k.add(item, nil)
	}
	if status != errSecSuccess {
		return fmt.Errorf("keychain write failed (OSStatus %d)", status)
	}
	return nil
}

func (s keychainSecrets) del(id string) error {
	k, err := loadKeychain()
	if err != nil {
		return err
	}
	kcMu.Lock()
	defer kcMu.Unlock()
	q, tmp := k.query(s.service, id)
	defer k.releaseAll(append(tmp, q))
	if status := k.remove(q); status != errSecSuccess && status != errSecItemNotFound {
		return fmt.Errorf("keychain delete failed (OSStatus %d)", status)
	}
	return nil
}

func (k *keychainAPI) releaseAll(refs []uintptr) {
	for _, r := range refs {
		if r != 0 {
			k.release(r)
		}
	}
}
