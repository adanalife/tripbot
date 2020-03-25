#
# Variables Configuration
#

variable "profile" {
  description = "Name of your profile inside ~/.aws/credentials"
}

variable "region" {
  default = "us-east-1"
  description = "Defines where your app should be deployed"
}

variable "application_environment" {
  description = "Deployment stage e.g. 'staging', 'production', 'test', 'integration'"
}


# RDS values

variable "username" {
  description = "Master username of the DB"
  type        = string
}

variable "password" {
  description = "Master password of the DB"
  type        = string
}

variable "database_name" {
  description = "Name of the database to be created"
  type        = string
}

#variable "name" {
#  description = "Name of the database"
#  type        = string
#}

variable "engine_name" {
  description = "Name of the database engine"
  type        = string
  default     = "postgres"
}


/* variable "family" { */
/*   description = "Family of the database" */
/*   type        = string */
/*   default     = "mysql5.7" */
/* } */

/* variable "port" { */
/*   description = "Port which the database should run on" */
/*   type        = number */
/*   default     = 5432 */
/* } */

/* variable "major_engine_version" { */
/*   description = "MAJOR.MINOR version of the DB engine" */
/*   type        = string */
/*   default     = "5.7" */
/* } */

/* variable "engine_version" { */
/*   description = "Version of the database to be launched" */
/*   default     = "5.7.21" */
/*   type        = string */
/* } */

variable "allocated_storage" {
  description = "Disk space to be allocated to the DB instance"
  type        = number
  default     = 5
}

/* variable "license_model" { */
/*   description = "License model of the DB instance" */
/*   type        = string */
/*   default     = "general-public-license" */
/* } */

