package tools_test

import (
	"testing"

	"github.com/zouxiaoliang/pocketbase-mcp-go/tests/testutil"
)

func TestListAuthMethods(t *testing.T) {
	testutil.RunRootTest(t, "TestListAuthMethods")
}

func TestAuthenticateWithOAuth2(t *testing.T) {
	testutil.RunRootTest(t, "TestAuthenticateWithOAuth2")
}

func TestAuthenticateWithOTP(t *testing.T) {
	testutil.RunRootTest(t, "TestAuthenticateWithOTP")
}

func TestAuthRefresh(t *testing.T) {
	testutil.RunRootTest(t, "TestAuthRefresh")
}

func TestRequestVerification(t *testing.T) {
	testutil.RunRootTest(t, "TestRequestVerification")
}

func TestConfirmVerification(t *testing.T) {
	testutil.RunRootTest(t, "TestConfirmVerification")
}

func TestRequestPasswordReset(t *testing.T) {
	testutil.RunRootTest(t, "TestRequestPasswordReset")
}

func TestConfirmPasswordReset(t *testing.T) {
	testutil.RunRootTest(t, "TestConfirmPasswordReset")
}

func TestRequestEmailChange(t *testing.T) {
	testutil.RunRootTest(t, "TestRequestEmailChange")
}

func TestConfirmEmailChange(t *testing.T) {
	testutil.RunRootTest(t, "TestConfirmEmailChange")
}

func TestImpersonateUser(t *testing.T) {
	testutil.RunRootTest(t, "TestImpersonateUser")
}

func TestImportData(t *testing.T) {
	testutil.RunRootTest(t, "TestImportData")
}
