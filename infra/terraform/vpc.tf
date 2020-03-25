# create a private VPC
resource "aws_vpc" "private" {
  cidr_block = "10.0.0.0/16"

  tags = map(
    "Name", "Private VPC - ${var.application_environment}",
  )
}

# create some subnets the private VPC
resource "aws_subnet" "private" {
  count = 2

  availability_zone = data.aws_availability_zones.available.names[count.index]
  cidr_block        = "10.0.${count.index}.0/24"
  vpc_id            = aws_vpc.private.id

  tags = map(
    "Name", "All-private VPC subnet - ${var.application_environment}",
    "kubernetes.io/cluster/${var.cluster-name}", "shared",
  )
}

# in order to access hosts in the private VPC, create an internet gateway
resource "aws_internet_gateway" "private" {
  vpc_id = aws_vpc.private.id

  tags = {
    Name = "Internet gateway - ${var.application_environment}"
  }
}

# create a route table for the private VPC and associate it with the gateway
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.private.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.private.id
  }
}

# associate the route table with the private subnets
resource "aws_route_table_association" "private" {
  count = 2

  subnet_id      = aws_subnet.private.*.id[count.index]
  route_table_id = aws_route_table.private.id
}
