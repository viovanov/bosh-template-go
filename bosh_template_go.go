package boshgotemplate

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	rbClassFileName               = "bosh_erb_renderer.rb"
	evaluationContextYAMLFileName = "evaluation_context.yaml"
	instanceInfoYAMLFileName      = "instance_info.yaml"
)

var (
	// RubyBinary is the name of the ruby binary. Can be an absolute path.
	RubyBinary = "ruby"
	// RubyGemBinary is the name of the ruby gem binary. Can be an absolute path.
	RubyGemBinary               = "gem"
	templateEvaluationContextRb = []byte(`
require "erb"
require "yaml"
require "bosh/template"
require 'fileutils'

if $0 == __FILE__
  context_path, spec_path, instance_path, src_path, dst_path = *ARGV

  puts "Context file: #{context_path}"
  puts "Instance file: #{instance_path}"
	puts "Spec file: #{spec_path}"
  puts "Template file: #{src_path}"
  puts "Output file: #{dst_path}"

	# Load the context hash
  context_hash = YAML.load_file(context_path)
	
	# Load the job spec
	job_spec = YAML.load_file(spec_path)

	# Load the instace info
	instance_info = YAML.load_file(instance_path)

  # Read the erb template
  begin
	perms = File.stat(src_path).mode
    template = Bosh::Template::Test::Template.new(job_spec, src_path)
  rescue Errno::ENOENT
		raise "failed to read template file #{src_path}"
  end

	# Build links
	links = []
	if context_hash['properties'] && context_hash['properties']['bosh_containerization'] && context_hash['properties']['bosh_containerization']['consumes']
		context_hash['properties']['bosh_containerization']['consumes'].each_pair do |name, link|
			next if link['instances'].empty?

			instances = []
			link['instances'].each do |link_instance|
				instances << Bosh::Template::Test::InstanceSpec.new(
					address:   link_instance['address'],
					az:        link_instance['az'],
					id:        link_instance['id'],
					index:     link_instance['index'],
					name:      link_instance['name'],
					bootstrap: link_instance['index'] == '0',
				)
			end
			links << Bosh::Template::Test::Link.new(name: name, instances: instances, properties: link['properties'])
		end
	end
	
	# Build instance
	instance = Bosh::Template::Test::InstanceSpec.new(
		address:    instance_info['address'],
		az:         instance_info['az'],
		bootstrap:  instance_info['index'] == '0',
		deployment: instance_info['deployment'],
		id:         instance_info['id'],
		index:      instance_info['index'],
		ip:         instance_info['ip'],
		name:       instance_info['name'],
		networks:   {'default' => {'ip' => instance_info['ip'],
															 'dns_record_name' => instance_info['address'],
															 # TODO: Do we need more, like netmask and gateway?
															 # https://github.com/cloudfoundry/bosh-agent/blob/master/agent/applier/applyspec/v1_apply_spec_test.go
															}},
	)

  # Process the Template
  output = template.render(context_hash['properties'], spec: instance, consumes: links)
  
  begin
		# Open the output file
		output_dir = File.dirname(dst_path)
		FileUtils.mkdir_p(output_dir)
		out_file = File.open(dst_path, 'w')

		# Write results to the output file
		out_file.write(output)

		# Set the appropriate permissions on the output file
		if File.basename(File.dirname(dst_path)) == 'bin'
			out_file.chmod(0755)
		else
			out_file.chmod(perms)
		end
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
	Properties map[string]interface{} `yaml:"properties"`
}

// InstanceInfo represents instance group runtime information
type InstanceInfo struct {
	Address    string `yaml:"address"`
	AZ         string `yaml:"az"`
	Deployment string `yaml:"deployment"`
	ID         string `yaml:"id"`
	Index      string `yaml:"index"`
	IP         string `yaml:"ip"`
	Name       string `yaml:"name"`
}

// ERBRenderer represents a BOSH Job erb template renderer
type ERBRenderer struct {
	EvaluationContext *EvaluationContext
	InstanceInfo      *InstanceInfo
	JobSpecFilePath   string
}

// NewERBRenderer creates a new ERBRenderer with an EvaluationContext
func NewERBRenderer(evaluationContext *EvaluationContext, instanceInfo *InstanceInfo, jobSpecFilePath string) *ERBRenderer {
	return &ERBRenderer{
		EvaluationContext: evaluationContext,
		JobSpecFilePath:   jobSpecFilePath,
		InstanceInfo:      instanceInfo,
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
	evalContextBytes, err := yaml.Marshal(e.EvaluationContext)
	if err != nil {
		return errors.Wrap(err, "failed to marshal the evaluation context")
	}
	evaluationContextYAMLFilePath := filepath.Join(tmpDir, evaluationContextYAMLFileName)
	err = ioutil.WriteFile(evaluationContextYAMLFilePath, evalContextBytes, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to write the evaluation context yaml file")
	}

	// Marshal instance information
	instanceInfoBytes, err := yaml.Marshal(e.InstanceInfo)
	if err != nil {
		return errors.Wrap(err, "failed to marshal instance runtime information")
	}
	instanceInfoYAMLFilePath := filepath.Join(tmpDir, instanceInfoYAMLFileName)
	err = ioutil.WriteFile(instanceInfoYAMLFilePath, instanceInfoBytes, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to write instance runtime information yaml file")
	}

	// Run rendering
	err = run(rbClassFilePath, evaluationContextYAMLFilePath, e.JobSpecFilePath, instanceInfoYAMLFilePath, inputFilePath, outputFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to render template")
	}

	return nil
}

func run(rubyClassFilePath, evaluationContextYAMLFilePath, jobSpecFilePath, instanceInfoYAMLFilePath, inputFilePath, outputFilePath string) error {
	cmd := exec.Command(RubyBinary, rubyClassFilePath, evaluationContextYAMLFilePath, jobSpecFilePath, instanceInfoYAMLFilePath, inputFilePath, outputFilePath)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		output := string(outputBytes)
		return errors.Wrapf(err, "rendering failed: %s", output)
	}
	return nil
}

func checkRubyAvailable() error {
	_, err := exec.LookPath(RubyBinary)
	if err != nil {
		return errors.Wrap(err, "rendering BOSH templates requires ruby, please install ruby and make sure it's in your PATH")
	}
	return nil
}

func checkBOSHTemplateGemAvailable() error {
	cmd := exec.Command(RubyGemBinary, "list", "-i", "bosh-template")
	outputBytes, err := cmd.CombinedOutput()

	if err != nil {
		output := string(outputBytes)
		return errors.Wrapf(err, "rendering BOSH templates requires the bosh-template ruby gem, please install it: %s ", output)
	}

	return nil
}
