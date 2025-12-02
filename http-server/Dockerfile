# --- Stage 1: Build the application ---
FROM golang:1.25-alpine AS builder

# Set the working directory inside the builder stage
WORKDIR /app

# Copy the go.mod and go.sum files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go application binary
# CGO_ENABLED=0 disables CGO, making the binary statically linked and compatible with the minimal 'scratch' base image
# -o app specifies the output filename for the executable
RUN CGO_ENABLED=0 go build -o app .


# --- Stage 2: Create the minimal runtime image ---
# Use 'scratch' for the smallest possible final image that contains only our binary
FROM scratch AS final

# Optional: Copy certificates for HTTPS/TLS communication if needed by the app
# COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Set a label to identify the author or project
LABEL maintainer="M. Oly Mahmud <mugdhodzs38@gmail.com>"

# Expose the necessary ports. This serves as documentation.
EXPOSE 8080/tcp
EXPOSE 8883/tcp

# Copy the compiled executable from the 'builder' stage into the 'final' stage
COPY --from=builder /app/app .

# Command to run the executable when the container starts
CMD ["./app"]
