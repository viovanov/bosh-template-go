package boshgotemplate

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	rbClassFileName               = "bosh_erb_renderer.rb"
	evaluationContextJSONFileName = "evaluation_context.json"
)

var (
	templateEvaluationContextRb = []byte(`
require "json"
require "erb"
require "yaml"
require "bosh/template"
require 'fileutils'

class KubeDNSEncoder
  def initialize(link_specs)
    @link_specs = link_specs
  end
end

evaluationContext = ARGV[0]
inputFile = ARGV[1]
outputFile = ARGV[2]


if $0 == __FILE__
  context_path, src_path, dst_path = *ARGV

  puts "Context file: #{context_path}"
  puts "Template file: #{src_path}"
  puts "Output file: #{dst_path}"

  context_hash = JSON.load(File.read(context_path))

  # TODO: set link specs here
  dns_encoder = KubeDNSEncoder.new({})

  # Read the erb template
  begin
	perms = File.stat(src_path).mode
	erb_template = ERB.new(File.read(src_path), nil, '-')
	erb_template.filename = src_path
  rescue Errno::ENOENT
	raise "failed to read template file #{src_path}"
  end

  # Create a BOSH evaluation context
  evaluation_context = Bosh::Template::EvaluationContext.new(context_hash, dns_encoder)
  # Process the Template
  output = erb_template.result(evaluation_context.get_binding)
  

  begin
	# Open the output file
	output_dir = File.dirname(dst_path)
	FileUtils.mkdir_p(output_dir)
	out_file = File.open(dst_path, 'w')
	# Write results to the output file
	out_file.write(output)
	# Set the appropriate permissions on the output file
	out_file.chmod(perms)
  rescue Errno::ENOENT, Errno::EACCES => e
  	out_file = nil
  	raise "failed to open output file #{dst_path}: #{e}"
  ensure
  	out_file.close unless out_file.nil?
  end
end
`)
)

// EvaluationContext is the context passed to the erb renderer
type EvaluationContext struct {
	Properties map[string]interface{} `json:"properties"`
	Networks   map[string]interface{} `json:"networks"`
}

// ERBRenderer represents a BOSH Job erb template renderer
type ERBRenderer struct {
	EvaluationContext *EvaluationContext
}

// NewERBRenderer creates a new ERBRenderer with an EvaluationContext
func NewERBRenderer(evaluationContext *EvaluationContext) *ERBRenderer {
	return &ERBRenderer{
		EvaluationContext: evaluationContext,
	}
}

// Render renders an erb file using an EvaluationContext
func (e *ERBRenderer) Render(inputFilePath, outputFilePath string) (returnErr error) {
	// Check that dependencies are available
	if err := checkRubyAvailable(); err != nil {
		return err
	}
	if err := checkBOSHTemplateGemAvailable(); err != nil {
		return err
	}

	// Create a temporary work directory
	tmpDir, err := ioutil.TempDir("", "bosh-erb-renderer")
	if err != nil {
		return errors.Wrap(err, "failed to create temporary directory in erb renderer")
	}
	defer func() {
		if err = os.RemoveAll(tmpDir); err != nil {
			returnErr = errors.Wrap(err, "failed to cleanup erb renderer temporary directory")
		}
	}()

	// Write the ruby class to a file
	rbClassFilePath := filepath.Join(tmpDir, rbClassFileName)
	err = ioutil.WriteFile(rbClassFilePath, templateEvaluationContextRb, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to write the rendering ruby class file")
	}

	// Marshal the evaluation context
	evalContextBytes, err := json.Marshal(e.EvaluationContext)
	if err != nil {
		return errors.Wrap(err, "failed to marshal the evaluation context")
	}
	evaluationContextJSONFilePath := filepath.Join(tmpDir, evaluationContextJSONFileName)
	err = ioutil.WriteFile(evaluationContextJSONFilePath, evalContextBytes, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to write the evaluation context json file")
	}

	// Run rendering
	err = run(rbClassFilePath, evaluationContextJSONFilePath, inputFilePath, outputFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to render template")
	}

	return nil
}

func run(rubyClassFilePath, evaluationContextJSONFilePath, inputFilePath, outputFilePath string) error {
	cmd := exec.Command("ruby", rubyClassFilePath, evaluationContextJSONFilePath, inputFilePath, outputFilePath)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		output := string(outputBytes)
		return errors.Wrapf(err, "rendering failed: %s", output)
	}
	return nil
}

func checkRubyAvailable() error {
	_, err := exec.LookPath("ruby")
	if err != nil {
		return errors.Wrap(err, "rendering BOSH templates requires ruby, please install ruby and make sure it's in your PATH")
	}
	return nil
}

func checkBOSHTemplateGemAvailable() error {
	cmd := exec.Command("gem", "list", "-i", "bosh-template")
	outputBytes, err := cmd.CombinedOutput()

	if err != nil {
		output := string(outputBytes)
		return errors.Wrapf(err, "rendering BOSH templates requires the bosh-template ruby gem, please install it: %s ", output)
	}

	return nil
}
