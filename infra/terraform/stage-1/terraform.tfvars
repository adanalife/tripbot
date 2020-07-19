environment = "stage"

# other account IDs
core_account_id = "729863845087"


vpc_dns_servers = ["172.31.0.2"]

# to generate these, we dedicated 172.31.128.0/18 to public subnets
# and then used the following tool to split into equally-sized chunks
# http://www.davidc.net/sites/default/subnets/subnets.html?network=172.31.128.0&mask=18&division=15.7231
# each public subnet can hold 2046 hosts
# (note that 172.31.176.0/21 and 172.31.184.0/21 are still available)
vpc_public_subnet_cidrs = [
  "172.31.128.0/21", // 172.31.128.1 - 172.31.135.254
  "172.31.136.0/21", // 172.31.136.1 - 172.31.143.254
  "172.31.144.0/21", // 172.31.144.1 - 172.31.151.254
  "172.31.152.0/21", // 172.31.152.1 - 172.31.159.254
  "172.31.160.0/21", // 172.31.160.1 - 172.31.167.254
  "172.31.168.0/21"  // 172.31.168.1 - 172.31.175.254
]

# to generate these, we dedicated 172.31.0.0/17 to private subnets
# and then used the following tool to split into equally-sized chunks
# http://www.davidc.net/sites/default/subnets/subnets.html?network=172.31.0.0&mask=17&division=15.7231
# each private subnet can hold 4094 hosts
vpc_private_subnet_cidrs = [
  "172.31.0.0/20",  // 172.31.0.1 - 172.31.15.254
  "172.31.16.0/20", // 172.31.16.1 - 172.31.31.254
  "172.31.32.0/20", // 172.31.32.1 - 172.31.47.254
  "172.31.48.0/20", // 172.31.48.1 - 172.31.63.254
  "172.31.64.0/20", // 172.31.64.1 - 172.31.79.254
  "172.31.80.0/20"  // 172.31.80.1 - 172.31.95.254
]

vpc_database_subnet_cidrs = [
  "172.31.96.0/20", // 172.31.96.1 - 172.31.111.254
  "172.31.112.0/20" // 172.31.112.1 - 172.31.127.254
]
