resource "aws_instance" "smart_home_web_server" {
  ami             = "ami-0fa91bc90632c73c9"
  instance_type   = "t3.micro"
  vpc_security_group_ids = [aws_security_group.iot_sg.id]
  region          = var.region
  subnet_id = aws_subnet.smart_home_subnet.id

  key_name = aws_key_pair.smart_home_key_pair.key_name

  associate_public_ip_address = true
  
  tags = {
    Name = "SmartHomeIoTInstance"
  }


  user_data = <<-EOF
                #!/bin/bash
                sudo apt-get update
                sudo apt-get install -y docker.io docker-compose
                source ~/.bashrc
                EOF
}


resource "tls_private_key" "key_generator" {
  algorithm   = "RSA"
  rsa_bits    = 4096 # Use 4096 bits for better security
}

resource "aws_key_pair" "smart_home_key_pair" {
  key_name   = "my-terraform-key" # Set your desired key pair name
  public_key = tls_private_key.key_generator.public_key_openssh
}

# Save the generated private key to a local file
resource "local_file" "private_key_file" {
  filename        = "secrets/${aws_key_pair.smart_home_key_pair.key_name}.pem"
  content         = tls_private_key.key_generator.private_key_pem
  file_permission = "0400" # Sets read-only permissions for the owner
}

