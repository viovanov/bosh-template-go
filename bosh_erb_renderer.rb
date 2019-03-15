
require "json"
require "erb"
require "yaml"
require "bosh/template"

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
  context_hash = JSON.load(File.read(context_path))

  # TODO: load link specs here
  dns_encoder = KubeDNSEncoder.new({})

  # Create a BOSH evaluation context
  evaluation_context = Bosh::Template::EvaluationContext.new(evaluationContext, dns_encoder)
  # Process the Template
  output = erb_template.result(evaluation_context.get_binding)
  

  begin
	# Open the output file
	output_dir = File.dirname(output_file_path)
	FileUtils.mkdir_p(output_dir)
	out_file = File.open(output_file_path, 'w')
	# Write results to the output file
	out_file.write(output)
	# Set the appropriate permissions on the output file
	out_file.chmod(perms)
  rescue Errno::ENOENT, Errno::EACCES => e
  	out_file = nil
  	raise "failed to open output file #{output_file_path}: #{e}"
  ensure
  	out_file.close unless out_file.nil?
  end
end


