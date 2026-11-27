#!/bin/sh
set -eu

: "${KAFKA_BOOTSTRAP_SERVER:=kafka:9092}"
: "${AUDIT_KAFKA_TOPIC:=plateau.audit.events}"
: "${KAFKA_TOPICS:=${AUDIT_KAFKA_TOPIC}:3:1}"
KAFKA_TOPICS_BIN="${KAFKA_TOPICS_BIN:-/opt/kafka/bin/kafka-topics.sh}"

validate_topic() {
  case "$1" in
  '' | '.' | '..' | *[!a-zA-Z0-9._-]*)
    echo "invalid Kafka topic name: $1" >&2
    exit 1
    ;;
  esac
}

validate_positive_int() {
  case "$2" in
  '' | 0* | *[!0-9]*)
    echo "invalid Kafka $1: $2" >&2
    exit 1
    ;;
  esac
}

for spec in $(printf '%s' "$KAFKA_TOPICS" | tr ',' ' '); do
  [ -n "$spec" ] || continue
  IFS=':' read -r topic partitions replication extra <<EOF
$spec
EOF
  if [ -n "${extra:-}" ]; then
    echo "invalid Kafka topic spec (expected topic[:partitions[:replication]]): $spec" >&2
    exit 1
  fi
  : "${partitions:=3}"
  : "${replication:=1}"

  validate_topic "$topic"
  validate_positive_int "partitions" "$partitions"
  validate_positive_int "replication-factor" "$replication"

  "$KAFKA_TOPICS_BIN" \
    --bootstrap-server "$KAFKA_BOOTSTRAP_SERVER" \
    --create \
    --if-not-exists \
    --topic "$topic" \
    --partitions "$partitions" \
    --replication-factor "$replication"
done
