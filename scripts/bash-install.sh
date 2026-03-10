#!/bin/bash

set -eu

cleanup_on_error() {
    if [[ -v TEMP_DIR ]]; then
        rm -f "${TEMP_DIR}/docker-compose.yml"
        docker compose -p "metrics" -f ${TEMP_DIR}/docker-compose.yml down -v || true
    fi
    echo -e "\e[31mInstallation failed\e[0m"
}

cleanup_on_exit(){
    if [[ -v TEMP_DIR ]]; then
        rm -f "${TEMP_DIR}/get-docker.sh"
        rm -f "${TEMP_DIR}/.env"
    fi
}

trap cleanup_on_error ERR
trap cleanup_on_exit EXIT

#Check root access
if [ $(id -u) -ne 0 ]; then
    echo -e "Script must be running as root"
    exit 1
fi


#Warning
read -p $'\e[33m!! WARNING !!\e[0m \nThis will remove existing containers and volumes of metrics service if they exist. Continue? [y/n]: ' choice
if  [ $choice = "n" ]; then
    exit 0
elif [ $choice != "y" ]; then
    echo "Invalid command"
    exit 1  
fi

#Specify folder for files for installation
read -p "Specify folder for temp files for installation: " TEMP_DIR
TEMP_DIR="${TEMP_DIR//[[:space:]]/}"
TEMP_DIR="${TEMP_DIR:-"."}"

echo ""
if ! pathchk "$TEMP_DIR" || [[ -f $TEMP_DIR ]]; then
    echo "Provided path is not correct or is not a directory"
    exit 1
fi

mkdir -p "${TEMP_DIR}"

#Check if docker installed
if ! $(command -v docker) &>/dev/null; then
    echo -e "Docker is not installed"
    read -p "Install docker? [y/n]: " choice
    if  [ $choice = "y" ]; then
        echo -e "Installing docker..."
        curl -fsSL https://get.docker.com -o "${TEMP_DIR}/get-docker.sh"
        chmod +x "${TEMP_DIR}/get-docker.sh"
        "${TEMP_DIR}/get-docker.sh"
        echo -e "Docker installed\nVersion: $(docker -v)\n"
    elif [ $choice = "n" ]; then
        exit 0
    else 
        echo "Invalid command"
        exit 1  
    fi
else 
    echo -e "Found installed Docker\nVersion: $(docker -v)\n"
fi

#Configure environment variables
read -p "Enter PostgreSQL username (default: postgres): " POSTGRES_USER
POSTGRES_USER="${POSTGRES_USER//[[:space:]]/}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"

read -sp "Enter PostgreSQL password (default: password): " POSTGRES_PASSWORD
echo ""
POSTGRES_PASSWORD="${POSTGRES_PASSWORD//[[:space:]]/}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-password}"

read -p "Enter PostgreSQL database name (default: postgres): " POSTGRES_DB
POSTGRES_DB="${POSTGRES_DB//[[:space:]]/}"
POSTGRES_DB="${POSTGRES_DB:-postgres}"

read -p "Enter Grafana username (default: admin): " GRAFANA_USER
GRAFANA_USER="${GRAFANA_USER//[[:space:]]/}"
GRAFANA_USER="${GRAFANA_USER:-admin}"

read -sp "Enter Grafana password (default: admin): " GRAFANA_PASSWORD
echo ""
GRAFANA_PASSWORD="${GRAFANA_PASSWORD//[[:space:]]/}"
GRAFANA_PASSWORD="${GRAFANA_PASSWORD:-admin}"
echo ""

touch "${TEMP_DIR}/.env"
chmod 600 "${TEMP_DIR}/.env"

echo "POSTGRES_USER=${POSTGRES_USER}" > "${TEMP_DIR}/.env"
echo "POSTGRES_PASSWORD=${POSTGRES_PASSWORD}" >> "${TEMP_DIR}/.env"
echo "POSTGRES_DB=${POSTGRES_DB}" >> "${TEMP_DIR}/.env"
echo "GRAFANA_USER=${GRAFANA_USER}" >> "${TEMP_DIR}/.env"
echo "GRAFANA_PASSWORD=${GRAFANA_PASSWORD}" >> "${TEMP_DIR}/.env"

#Fetch docker-compose file
echo "Fetching docker-compose file"s
curl -fsSL https://raw.githubusercontent.com/Cardinal87/Metric_Collector/main/infrastructure/docker-compose.yml -o "${TEMP_DIR}/docker-compose.yml"

#Remove volumes and previous containers if exists
echo "Cleaning up old containers and volumes..."
docker compose -p "metrics" -f ${TEMP_DIR}/docker-compose.yml down -v || true

#Start containers
echo "Starting containers"
docker compose -p "metrics" --env-file "${TEMP_DIR}/.env" -f "${TEMP_DIR}/docker-compose.yml" up -d

echo "Starting database..."
until docker exec metrics-db-1 pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}; do
    sleep 1
done

#Create read-only user for grafana
echo "Creating special user for grafana..."
docker exec metrics-db-1 psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} -v ON_ERROR_STOP=1 -c "
    create user grafana_user with password '${GRAFANA_PASSWORD}' ;
    grant connect on database ${POSTGRES_DB} to grafana_user;
    grant usage on schema public to grafana_user;
    grant select on all tables in schema public to grafana_user;
    alter default privileges in schema public grant select on tables to grafana_user;
    "

echo -e "\n"
echo "======================================="
echo "======================================="
echo -e "\n"

echo -e "\e[32mInstallation completed\e[0m"
echo ""
echo "Check containers status with: \"docker ps\" command"
echo ""
echo "Create \"config.json\" according to https://github.com/Cardinal87/Metric_Collector/blob/main/server/config_schema.json"
echo ""
echo -e "Your database connection string: \e[32m\"postgresql://postgres:${POSTGRES_PASSWORD}@localhost:5432/${POSTGRES_DB}?sslmode=disable\"\e[0m"
echo -e "Your credentials to connect the TimeScaleDB datasource to Grafana:"
echo -e "\tusername: \e[32mgrafana_user\e[0m"
echo -e "\tpassword: \e[32m${GRAFANA_PASSWORD}\e[0m"
echo -e "\thost: \e[32mdb:5432\e[0m"
echo -e "\database: \e[32m${POSTGRES_DB}\e[0m"



