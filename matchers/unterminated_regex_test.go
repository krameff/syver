package matchers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// brokenRegexPatterns are patterns written as regexes whose closing "/" is
// missing or is followed by something other than flags, so the matcher reads
// them as plain text. The first group is taken from published audit content;
// two keep the "..." of a truncated original, which changes nothing here.
var brokenRegexPatterns = []string{
	`!/^.*:[a-w][y-z]:.*`,
	`!/^UMASK 0[1-6]{2;}`,
	`!/^cipher@/(lib|open)ssh(-server|-client)?\s*=\s*([^#\n\r]+)?-CBC\b`,
	`!/[Ll]inux`,
	`!/.*needs update`,
	`!/^[[tT]imed[lL]ogin[eE]nable=true`,
	`!/^SELINUX=disabled`,
	`!/^\s*etm\b`,
	`!/^space_left = ([0-9]|1[0-9]|2[0-4])%`,
	`!/^port [1-9]{1,9}`,
	`!/^cmdport [1-9]{1,9}`,
	`!/^(?i)cipher@/(lib|open)ssh(-server|-client)?\s*=\s*([^#\n\r]+)?-chacha-poly1305\b`,
	`!/^Please.*`,
	`/ServerKeyFile=.*.pem`,
	`/ServerCertificateFile=.*.pem`,
	`/TrustedCertificateFile=.*.pem`,
	`/^.*:-a always,exit -F arch=b64 -S truncate,ftruncate,creat,open,openat,open_by_handle_...`,
	`/^password\s+(required|requisite|sufficient)\s+pam_unix.so\s+sha512.*rounds=10000`,
	`/^password_pbkdf2`,
	`/^ForwardToSyslog=yes`,
	`/^[[tT]imed[lL]ogin[eE]nable=false`,
	`/^password\s+requisite\s+pam_pwhistory.so remember=5 retry=3/'`,
	`/^password\s+requisite\s+pam_pwhistory.so use_authtok remember=5 retry=3/'`,
	`/^auth required pam_faillock.so authfail dir={{ .Vars.rhel8stig_pam_faillock_dir }}`,
	`/^matchrule =,SAN..*EDIPI@mil`,
	`/^.*.conf\:\*\.\* {{ .Vars.rhel8stig_remotelog_server_protocol }}\[{{ .Vars.rhel8stig_r...`,
	`/^domains =`,
	`/\s\/var\/log\/audit\s.*noexec`,
	`/^\s*password_pbkdf2.*\s{{ .Vars.suse15cis_bootloader_password_hash }}\`,
	`/^\s*password_pbkdf2.*\s{{ .Vars.suse16cis_bootloader_password_hash }}\`,
	`/^Policy:.*:\s\b([1-9]|10)\b`,
}

// singleSignalPatterns each carry exactly one kind of regex syntax, so a rule
// that stops recognising that kind fails its row.
var singleSignalPatterns = map[string]string{
	"leading anchor":     `/^PermitRootLogin`,
	"trailing anchor":    `/foo$`,
	"regex escape":       `!/PermitRootLogin\s+yes`,
	"dot-star":           `/ServerKeyFile=.*.pem`,
	"character class":    `!/[Ll]inux`,
	"group":              `/(?i)foo`,
	"alternation":        `/foo|bar`,
	"bounded quantifier": `/x{2}`,
}

// trailingPatterns have a closing "/" followed by something that is not
// i, m or s flags; trailing is what follows the last "/".
var trailingPatterns = []struct {
	pattern, trailing string
}{
	{`/^x/'`, `'`},
	{`/^x/g`, `g`},
	{`/^x/ `, ` `},
	{`!/foo.*bar/ix`, `ix`},
	{`!/^cipher@/(lib|open)ssh(-server|-client)?\s*=`, `(lib|open)ssh(-server|-client)?\s*=`},
}

// allBrokenPatterns is every pattern UnterminatedRegex must flag.
func allBrokenPatterns() []string {
	out := append([]string{}, brokenRegexPatterns...)
	for _, p := range singleSignalPatterns {
		out = append(out, p)
	}
	for _, tp := range trailingPatterns {
		out = append(out, tp.pattern)
	}
	return out
}

func TestUnterminatedRegex_BrokenPatternsWarn(t *testing.T) {
	for _, p := range brokenRegexPatterns {
		_, ok := UnterminatedRegex(p)
		assert.True(t, ok, "%s should be flagged", p)
	}
	for name, p := range singleSignalPatterns {
		trailing, ok := UnterminatedRegex(p)
		assert.True(t, ok, "%s (%s only) should be flagged", p, name)
		assert.Equal(t, "", trailing, "%s has no later /", p)
	}
	for _, tp := range trailingPatterns {
		trailing, ok := UnterminatedRegex(tp.pattern)
		assert.True(t, ok, "%s should be flagged", tp.pattern)
		assert.Equal(t, tp.trailing, trailing, "text after the last / of %s", tp.pattern)
	}
}

func TestUnterminatedRegex_PathsAndTextDoNotWarn(t *testing.T) {
	quiet := []string{
		// paths and ordinary text
		`/etc/passwd`,
		`/usr/bin/foo`,
		`/etc/sudoers.d/*`,
		`/etc/profile.d/*.sh`,
		`/home`,
		`/var/log/faillock`,
		`/org/gnome/desktop/session/idle-delay`,
		`/sbin/auditctl p+i+n+u+g+s+b+acl+xattrs+sha512`,
		`/opt/app-1.2.3/bin`,
		`/api/v1/users?id=1&name=x`,
		`/`,
		`!/`,
		`//`,
		`/(`,
		`/dev/mapper/rhel-tmp on /tmp type xfs (rw,nosuid,nodev,noexec,relatime,seclabel)`,
		`/srv/nfs 192.168.0.0/24(rw,sync,no_root_squash)`,
		`/srv/nfs *(ro)`,
		`/lib/modules/$(uname -r)/kernel`,
		`/opt/app (copy)`,
		`/usr/sbin/logrotate /etc/logrotate.conf > /dev/null 2>&1 || true`,
		`/usr/bin/sudo -l | grep NOPASSWD`,
		`/etc/cron.*`,
		`/etc/rc.d/rc.*`,
		`/home/*/.*`,
		`/var/log/.*`,
		`!/var/log/.*`,
		`/usr/local/bin/script.sh \`,
		// slash-only patterns with no regex syntax: not told apart from paths
		`/faillog_t`,
		`!/MANUAL`,
		`/password_pbkdf2`,
		`/foo/gi`,
		// escaped, so already text on purpose
		`\/^x`,
		`!\/^x`,
		// no leading slash
		`^foo`,
		// real regexes
		`/^x/`,
		`!/^x/i`,
		`/a/b/`,
	}
	for _, p := range quiet {
		trailing, ok := UnterminatedRegex(p)
		assert.False(t, ok, "%s should not be flagged", p)
		assert.Equal(t, "", trailing, "trailing is empty when ok is false: %s", p)
	}
}

func TestEscapeAsText_IsTheSameTextAndDoesNotWarn(t *testing.T) {
	for _, p := range allBrokenPatterns() {
		esc := EscapeAsText(p)
		_, ok := UnterminatedRegex(esc)
		assert.False(t, ok, "the escape %s of %s must not warn", esc, p)
		assert.False(t, isRegexPattern(esc), "the escape %s must stay text", esc)

		want, got := newStringPattern(p), newStringPattern(esc)
		assert.Equal(t, want.cleanPattern, got.cleanPattern, "escape of %s matches different text", p)
		assert.Equal(t, want.inverse, got.inverse, "escape of %s changes inverse", p)
	}
	assert.Equal(t, `!\/x`, EscapeAsText(`!/x`))
	assert.Equal(t, `\/x`, EscapeAsText(`/x`))
}
