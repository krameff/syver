package syver

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/logutils"
	"github.com/krameff/syver/util"
)

func setLogLevel(c *util.Config) error {
	return setLogLevelTo(c, os.Stderr)
}

// setLogLevelTo is setLogLevel with the destination exposed, so a test can put
// the real level filter between the logger and a buffer instead of bypassing
// it with log.SetOutput.
func setLogLevelTo(c *util.Config, w io.Writer) error {
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel("INFO"),
		Writer:   w,
	}
	log.SetFlags(0) // Turn off standard timestamp flags
	log.SetOutput(&timestampedWriter{filter})
	for _, lvl := range filter.Levels {
		cLvl := strings.ToUpper(c.LogLevel)
		if string(lvl) == cLvl {
			filter.MinLevel = lvl
			log.Printf("[DEBUG] Setting log level to %v", cLvl)
			return nil
		}
	}
	return fmt.Errorf("unsupported log level: %s", c.LogLevel)
}

type timestampedWriter struct {
	wrappedWriter io.Writer
}

func (t *timestampedWriter) Write(b []byte) (int, error) {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	return fmt.Fprintf(t.wrappedWriter, "%s %s", timestamp, b)
}
