// Code generated from go-difflib v1.0.0 output, the oracle for FEAT-027.
// DO NOT regenerate from the new renderer: that would test it against itself.

package outputs

var goDifflibOracle = []struct{ name, want, got, diff string }{
	{"captured 01", "2", "1", "--- test\n+++ actual\n@@ -1 +1 @@\n-2\n+1\n"},
	{"captured 02", "this is a failing test", "this is a test", "--- test\n+++ actual\n@@ -1 +1 @@\n-this is a failing test\n+this is a test\n"},
	{"captured 03", "x", "foo", "--- test\n+++ actual\n@@ -1 +1 @@\n-x\n+foo\n"},
	{"captured 04", "g", "foo", "--- test\n+++ actual\n@@ -1 +1 @@\n-g\n+foo\n"},
	{"captured 05", "this is a test1\nthis is a test9\nthis is a test3\n", "this is a test1\nthis is a test2\nthis is a test3\n", "--- test\n+++ actual\n@@ -1,3 +1,3 @@\n this is a test1\n-this is a test9\n+this is a test2\n this is a test3\n"},
	{"captured 06", "this is a test9\n", "this is a test1\n", "--- test\n+++ actual\n@@ -1,2 +1,2 @@\n-this is a test9\n+this is a test1\n \n"},
	{"captured 07", "^foo", "this is a test", "--- test\n+++ actual\n@@ -1 +1 @@\n-^foo\n+this is a test\n"},
	{"captured 08", "oo", "foo", "--- test\n+++ actual\n@@ -1 +1 @@\n-oo\n+foo\n"},
	{"captured 09", "f", "foo", "--- test\n+++ actual\n@@ -1 +1 @@\n-f\n+foo\n"},
	{"captured 10", "o", "foo", "--- test\n+++ actual\n@@ -1 +1 @@\n-o\n+foo\n"},
	{"captured 11", "^this", "this is a test", "--- test\n+++ actual\n@@ -1 +1 @@\n-^this\n+this is a test\n"},
	{"captured 12", "x", "bar", "--- test\n+++ actual\n@@ -1 +1 @@\n-x\n+bar\n"},
	{"captured 13", "cowx", "cow", "--- test\n+++ actual\n@@ -1 +1 @@\n-cowx\n+cow\n"},
	{"captured 14", "129", "128", "--- test\n+++ actual\n@@ -1 +1 @@\n-129\n+128\n"},
	{"captured 15", "not-cool", "cool", "--- test\n+++ actual\n@@ -1 +1 @@\n-not-cool\n+cool\n"},
	{"captured 16", "b", "a", "--- test\n+++ actual\n@@ -1 +1 @@\n-b\n+a\n"},
	{"synthetic 01", "PermitRootLogin no\nPasswordAuthentication no\nX11Forwarding no\nMaxAuthTries 4\nClientAliveInterval 300\nClientAliveCountMax 3\nLoginGraceTime 60\nBanner /etc/issue.net\n", "PermitRootLogin yes\nPasswordAuthentication no\nX11Forwarding no\nMaxAuthTries 4\nClientAliveInterval 300\nClientAliveCountMax 0\nLoginGraceTime 60\nBanner /etc/issue.net\n", "--- test\n+++ actual\n@@ -1,2 +1,2 @@\n-PermitRootLogin no\n+PermitRootLogin yes\n PasswordAuthentication no\n@@ -5,3 +5,3 @@\n ClientAliveInterval 300\n-ClientAliveCountMax 3\n+ClientAliveCountMax 0\n LoginGraceTime 60\n"},
	{"synthetic 02", "a\nb\nc\nd\ne\nf\ng\n", "a\nc\nd\nb\ne\nf\ng\nh\n", "--- test\n+++ actual\n@@ -1,5 +1,5 @@\n a\n-b\n c\n d\n+b\n e\n@@ -7,2 +7,3 @@\n g\n+h\n \n"},
	{"synthetic 03", "{\n  \"name\": \"web\",\n  \"port\": 80,\n  \"tls\": {\n    \"enabled\": true\n  },\n  \"workers\": 4\n}\n", "{\n  \"name\": \"web\",\n  \"port\": 8080,\n  \"workers\": 4,\n  \"tls\": {\n    \"enabled\": false\n  }\n}\n", "--- test\n+++ actual\n@@ -2,7 +2,7 @@\n   \"name\": \"web\",\n-  \"port\": 80,\n+  \"port\": 8080,\n+  \"workers\": 4,\n   \"tls\": {\n-    \"enabled\": true\n-  },\n-  \"workers\": 4\n+    \"enabled\": false\n+  }\n }\n"},
	{"edge empty want", "", "x\n", "--- test\n+++ actual\n@@ -1 +1,2 @@\n+x\n \n"},
	{"edge empty got", "x\n", "", "--- test\n+++ actual\n@@ -1,2 +1 @@\n-x\n \n"},
	{"edge no trailing newline on one side", "a\nb", "a\nb\n", "--- test\n+++ actual\n@@ -2 +2,2 @@\n b\n+\n"},
	{"edge no trailing newline both sides", "a\nb", "a\nc", "--- test\n+++ actual\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n"},
	{"edge CRLF", "a\r\nb\r\nc\r\n", "a\r\nB\r\nc\r\n", "--- test\n+++ actual\n@@ -1,3 +1,3 @@\n a\r\n-b\r\n+B\r\n c\r\n"},
	{"edge multibyte", "héllo\n日本\n🙂\n", "héllo\n日本語\n🙂\n", "--- test\n+++ actual\n@@ -1,3 +1,3 @@\n héllo\n-日本\n+日本語\n 🙂\n"},
	{"edge first and last line", "1\n2\n3\n4\n5\n6\n", "one\n2\n3\n4\n5\nsix\n", "--- test\n+++ actual\n@@ -1,2 +1,2 @@\n-1\n+one\n 2\n@@ -5,3 +5,3 @@\n 5\n-6\n+six\n \n"},
	{"edge gap of 2 merges", "a\nb\nc\nd\ne\n", "A\nb\nc\nD\ne\n", "--- test\n+++ actual\n@@ -1,5 +1,5 @@\n-a\n+A\n b\n c\n-d\n+D\n e\n"},
	{"edge gap of 3 splits", "a\nb\nc\nd\ne\nf\n", "A\nb\nc\nd\nE\nf\n", "--- test\n+++ actual\n@@ -1,2 +1,2 @@\n-a\n+A\n b\n@@ -4,3 +4,3 @@\n d\n-e\n+E\n f\n"},
	{"edge only additions", "a\nb\n", "a\nx\nb\ny\n", "--- test\n+++ actual\n@@ -1,3 +1,5 @@\n a\n+x\n b\n+y\n \n"},
	{"edge only deletions", "a\nx\nb\ny\n", "a\nb\n", "--- test\n+++ actual\n@@ -1,5 +1,3 @@\n a\n-x\n b\n-y\n \n"},
}

// go-difflib on largeDiffInputs(): 251 bytes.
const goDifflibLargeSHA256 = "cf02b0906c29110779cd32c335dad373e738d82d7a38242e985cc0bc45f4ccea"
