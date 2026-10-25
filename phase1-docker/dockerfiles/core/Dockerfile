# ---------- Stage 1: build ----------
# Must be a Debian/Ubuntu-based image (NOT alpine): the protobuf Maven plugin
# downloads glibc-linked protoc / protoc-gen-grpc-java executables at build time.
FROM maven:3.6.3-openjdk-17 AS build
WORKDIR /build

COPY pom.xml .
COPY src ./src

# BuildKit cache mount keeps ~/.m2 between builds, so dependencies are
# downloaded only once (needs internet / a working mirror on the first build)
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B clean package -DskipTests

# ---------- Stage 2: runtime ----------
FROM eclipse-temurin:17-jre-alpine
RUN addgroup -S app && adduser -S -G app app

WORKDIR /app
# No "RUN chown" here on purpose: it would create a second layer containing a full
# copy of the jar. The file is world-readable, which is all the JVM needs.
COPY --from=build /build/target/fum-cloud-notification-core-0.0.1-SNAPSHOT.jar ./app.jar

USER app

# Lets the JVM size its heap from the container memory limit (important in K8s)
ENV JAVA_OPTS="-XX:MaxRAMPercentage=75.0"

EXPOSE 8080

# Config is injected through env vars, e.g.:
#   PROJECT_DATASOURCE_JDBC_URL / _USERNAME / _PASS / _SCHEMA
#   ACTIVEMQ_URL / _USERNAME / _PASSWORD
#   AUTH_GRPC_ADDRESS / AUTH_GRPC_PORT / AUTH_SERVICE_BASE_URL
ENTRYPOINT ["sh", "-c", "exec java $JAVA_OPTS -jar app.jar"]
