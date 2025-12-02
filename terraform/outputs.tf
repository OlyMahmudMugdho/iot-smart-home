output "certificate_pem" {
  value     = aws_iot_certificate.device.certificate_pem
  sensitive = true
}

output "private_key" {
  value     = aws_iot_certificate.device.private_key
  sensitive = true
}

output "public_key" {
  value     = aws_iot_certificate.device.public_key
  sensitive = true
}

output "iot_endpoint_address" {
  value = data.aws_iot_endpoint.iot_data_ats_endpoint.endpoint_address
  description = "The address of the AWS IoT Data-ATS endpoint."
}


# Output the name of the created key pair
output "key_pair_name" {
  value = aws_key_pair.smart_home_key_pair.key_name
}

# output public IP of the instance
output "instance_public_ip" {
  value = aws_instance.smart_home_web_server.public_ip
}

# output dns name of the instance
output "instance_public_dns" {
  value = aws_instance.smart_home_web_server.public_dns
}