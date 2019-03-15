package boshgotemplate

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// testDir returns the path to the project
func testDir() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	return testDirSearch(dir)
}

// testDir recursively searches for the project root dir
func testDirSearch(dir string) string {
	// we've reached some dir that can't be this project
	if len(dir) < len("bosh-template-go") {
		panic("your current working dir is not inside the project")
	}

	// if the dir name is correct and there's a .git, we've found it
	if filepath.Base(dir) == "bosh-template-go" {
		if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
			return dir
		}
	}

	// keep searching
	return testDirSearch(filepath.Dir(dir))
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestRenderOK(t *testing.T) {
	// Arrange
	assert := assert.New(t)
	erbFile := filepath.Join(testDir(), "assets", "simple_test.erb")
	erbRenderer := NewERBRenderer(&EvaluationContext{
		Properties: map[string]interface{}{
			"foo": "bar",
		},
	})
	outDir, err := ioutil.TempDir("", "bosh-erb-render")
	assert.NoError(err)
	outFile := filepath.Join(outDir, "output")

	// Act
	err = erbRenderer.Render(erbFile, outFile)
	assert.NoError(err)

	output, err := ioutil.ReadFile(outFile)

	// Assert
	assert.NoError(err)
	assert.Equal("bar", string(output))
}
