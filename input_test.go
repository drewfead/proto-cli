package protocli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cliv1 "github.com/drewfead/proto-cli/proto/cli/v1"
)

func TestProtoJSONInput(t *testing.T) {
	f := ProtoJSONInput()
	assert.Equal(t, "json", f.Name())
	assert.Equal(t, []string{".json"}, f.Extensions())

	msg := &cliv1.CommandOptions{}
	err := f.Unmarshal([]byte(`{"name": "test-cmd", "description": "A test command"}`), msg)
	require.NoError(t, err)
	assert.Equal(t, "test-cmd", msg.GetName())
	assert.Equal(t, "A test command", msg.GetDescription())
}

func TestProtoJSONInput_InvalidJSON(t *testing.T) {
	f := ProtoJSONInput()
	msg := &cliv1.CommandOptions{}
	err := f.Unmarshal([]byte(`not json`), msg)
	assert.Error(t, err)
}

func TestYAMLInput(t *testing.T) {
	f := YAMLInput()
	assert.Equal(t, "yaml", f.Name())
	assert.Equal(t, []string{".yaml", ".yml"}, f.Extensions())

	msg := &cliv1.CommandOptions{}
	err := f.Unmarshal([]byte("name: test-cmd\ndescription: A test command\n"), msg)
	require.NoError(t, err)
	assert.Equal(t, "test-cmd", msg.GetName())
	assert.Equal(t, "A test command", msg.GetDescription())
}

func TestYAMLInput_InvalidYAML(t *testing.T) {
	f := YAMLInput()
	msg := &cliv1.CommandOptions{}
	// Binary garbage that fails YAML parsing
	err := f.Unmarshal([]byte{0x00, 0x01, 0xFF, 0xFE}, msg)
	assert.Error(t, err)
}

func TestDefaultInputFormats(t *testing.T) {
	formats := DefaultInputFormats()
	require.Len(t, formats, 2)
	assert.Equal(t, "json", formats[0].Name())
	assert.Equal(t, "yaml", formats[1].Name())
}

func TestReadInputFile_JSON_ByExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "from-json"}`), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "from-json", msg.GetName())
}

func TestReadInputFile_YAML_ByExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: from-yaml\n"), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "from-yaml", msg.GetName())
}

func TestReadInputFile_YML_ByExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.yml")
	require.NoError(t, os.WriteFile(path, []byte("name: from-yml\n"), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "from-yml", msg.GetName())
}

func TestReadInputFile_ExplicitFormatName(t *testing.T) {
	dir := t.TempDir()
	// File has .txt extension but we explicitly specify yaml format
	path := filepath.Join(dir, "request.txt")
	require.NoError(t, os.WriteFile(path, []byte("name: explicit-yaml\n"), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "yaml", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "explicit-yaml", msg.GetName())
}

func TestReadInputFile_ExplicitFormatName_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.txt")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "explicit-json"}`), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "json", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "explicit-json", msg.GetName())
}

func TestReadInputFile_ExplicitFormatName_Unknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.txt")
	require.NoError(t, os.WriteFile(path, []byte("name: test\n"), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "xml", DefaultInputFormats(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown input format")
	assert.Contains(t, err.Error(), "xml")
}

func TestReadInputFile_TryAll_Fallback(t *testing.T) {
	dir := t.TempDir()
	// No extension match, but valid JSON content
	path := filepath.Join(dir, "request.dat")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "fallback"}`), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "fallback", msg.GetName())
}

func TestReadInputFile_TryAll_AllFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.dat")
	// Binary content that is invalid for both JSON and YAML→protojson
	require.NoError(t, os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}, 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no format matched")
}

func TestReadInputFile_FileNotFound(t *testing.T) {
	msg := &cliv1.CommandOptions{}
	err := ReadInputFile("/nonexistent/path/request.json", "", DefaultInputFormats(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read input file")
}

func TestReadInputFile_MultipleFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	jsonContent := `{
		"name": "test-cmd",
		"description": "A test",
		"longDescription": "Detailed description",
		"localOnly": true
	}`
	require.NoError(t, os.WriteFile(path, []byte(jsonContent), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "test-cmd", msg.GetName())
	assert.Equal(t, "A test", msg.GetDescription())
	assert.Equal(t, "Detailed description", msg.GetLongDescription())
	assert.True(t, msg.GetLocalOnly())
}

func TestReadInputFile_MultipleFields_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.yaml")
	yamlContent := `name: test-cmd
description: A test
longDescription: Detailed description
localOnly: true
`
	require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	require.NoError(t, err)
	assert.Equal(t, "test-cmd", msg.GetName())
	assert.Equal(t, "A test", msg.GetDescription())
	assert.Equal(t, "Detailed description", msg.GetLongDescription())
	assert.True(t, msg.GetLocalOnly())
}

// customInputFormat is a test implementation of InputFormat.
type customInputFormat struct {
	name       string
	extensions []string
	unmarshal  func([]byte, proto.Message) error
}

func (f *customInputFormat) Name() string        { return f.name }
func (f *customInputFormat) Extensions() []string { return f.extensions }
func (f *customInputFormat) Unmarshal(data []byte, msg proto.Message) error {
	return f.unmarshal(data, msg)
}

func TestReadInputFile_CustomFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.custom")
	require.NoError(t, os.WriteFile(path, []byte("custom-content"), 0o644))

	custom := &customInputFormat{
		name:       "custom",
		extensions: []string{".custom"},
		unmarshal: func(_ []byte, msg proto.Message) error {
			cmd, ok := msg.(*cliv1.CommandOptions)
			if !ok {
				return nil
			}
			cmd.Name = "from-custom-format"
			return nil
		},
	}

	msg := &cliv1.CommandOptions{}
	formats := append(DefaultInputFormats(), custom)
	err := ReadInputFile(path, "", formats, msg)
	require.NoError(t, err)
	assert.Equal(t, "from-custom-format", msg.GetName())
}

func TestReadInputFile_ExplicitFormatError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	// Write invalid JSON content
	require.NoError(t, os.WriteFile(path, []byte("not-json"), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "json", DefaultInputFormats(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal input file")
}

func TestReadInputFile_ExtensionMatchError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	// Write invalid JSON content but with .json extension
	require.NoError(t, os.WriteFile(path, []byte("{invalid"), 0o644))

	msg := &cliv1.CommandOptions{}
	err := ReadInputFile(path, "", DefaultInputFormats(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal input file")
}
