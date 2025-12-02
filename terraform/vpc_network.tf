resource "aws_vpc" "smart_home_vpc" {
  cidr_block = "10.0.0.0/16"
  region     = var.region
  tags = {
    Name = "smart_home_vpc"
  }
}

resource "aws_subnet" "smart_home_subnet" {
  vpc_id            = aws_vpc.smart_home_vpc.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "${var.region}a"
  tags = {
    Name = "smart_home_subnet"
  }
}

resource "aws_internet_gateway" "smart_home_igw" {
  vpc_id = aws_vpc.smart_home_vpc.id
  tags = {
    Name = "smart_home_igw"
  }
}

resource "aws_route_table" "smart_home_rt" {
  vpc_id = aws_vpc.smart_home_vpc.id
  tags = {
    Name = "smart_home_route_table"
  }
}


resource "aws_route" "public_internet_access" {
  route_table_id         = aws_route_table.smart_home_rt.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.smart_home_igw.id
}

resource "aws_route_table_association" "smart_home_subnet_association" {
  subnet_id      = aws_subnet.smart_home_subnet.id
  route_table_id = aws_route_table.smart_home_rt.id
}


resource "aws_security_group" "iot_sg" {
  name        = "smart_home_security_group"
  description = "Security group for IoT instance"
  vpc_id      = aws_vpc.smart_home_vpc.id
}

resource "aws_security_group_rule" "allow_http_traffic" {
  type              = "ingress"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  security_group_id = aws_security_group.iot_sg.id
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "Allow HTTP traffic from anywhere"
}

resource "aws_security_group_rule" "allow_https_traffic" {
  type              = "ingress"
  from_port         = 443
  to_port           = 443
  protocol          = "tcp"
  security_group_id = aws_security_group.iot_sg.id
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "Allow HTTPS traffic from anywhere"
}


resource "aws_security_group_rule" "allow_ssh_traffic" {
  type              = "ingress"
  from_port         = 22
  to_port           = 22
  protocol          = "tcp"
  security_group_id = aws_security_group.iot_sg.id
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "Allow SSH traffic from anywhere"
}

resource "aws_security_group_rule" "allow_iot_traffic" {
  type              = "ingress"
  from_port         = 8883
  to_port           = 8883
  protocol          = "tcp"
  security_group_id = aws_security_group.iot_sg.id
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "Allow MQTT traffic from IoT devices"
}

resource "aws_security_group_rule" "allow_outbound_traffic" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  security_group_id = aws_security_group.iot_sg.id
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "Allow all outbound traffic"
}

