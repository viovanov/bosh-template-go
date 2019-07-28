require "bosh/template"
require "erb"
require "fileutils"
require "json"
require "yaml"

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
  job_spec['properties'] = {} if job_spec['properties'].nil?

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
          bootstrap: link_instance['bootstrap'],
          id:        link_instance['id'],
          index:     link_instance['index'],
          name:      link_instance['name'],
        )
      end
      links << Bosh::Template::Test::Link.new(
        name: name,
        address: link['address'],
        instances: instances,
        properties: link['properties'],
      )
    end
  end

  # Build instance
  instance = Bosh::Template::Test::InstanceSpec.new(
    address:    instance_info['address'],
    az:         instance_info['az'],
    bootstrap:  instance_info['bootstrap'],
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
