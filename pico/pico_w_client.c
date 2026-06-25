#include <stdio.h>
#include <string.h>
#include "pico/stdlib.h"
#include "pico/cyw43_arch.h"
#include "lwip/tcp.h"
#include "lwip/dns.h"
#include "waveshare/DEV_Config.h"
#include "waveshare/EPD_2in9_V2.h"

// Include config (user must create config.h from config.h.example)
#ifdef HAS_CONFIG_H
#include "config.h"
#else
// Fallbacks for testing
#define WIFI_SSID       "TEST_SSID"
#define WIFI_PASSWORD   "TEST_PASSWORD"
#define SERVER_IP       "192.168.1.100"
#define SERVER_PORT     "8080"
#define REFRESH_INTERVAL_MIN 15
#endif

#define DISPLAY_BUFFER_SIZE 4736

typedef enum {
    STATE_DISCONNECTED,
    STATE_CONNECTING,
    STATE_CONNECTED,
    STATE_RECEIVING,
    STATE_COMPLETED,
    STATE_FAILED
} client_state_t;

typedef struct {
    struct tcp_pcb *pcb;
    client_state_t state;
    uint8_t buffer[DISPLAY_BUFFER_SIZE];
    int bytes_received;
    bool headers_done;
    int header_search_idx;
} http_client_t;

// Global client structure
static http_client_t client;

// Callbacks for lwIP TCP
static void close_client_connection(struct tcp_pcb *tpcb) {
    tcp_arg(tpcb, NULL);
    tcp_sent(tpcb, NULL);
    tcp_recv(tpcb, NULL);
    tcp_err(tpcb, NULL);
    tcp_close(tpcb);
    client.pcb = NULL;
}

static void err_callback(void *arg, err_t err) {
    printf("TCP connection error: %d\n", err);
    client.state = STATE_FAILED;
    client.pcb = NULL;
}

static err_t recv_callback(void *arg, struct tcp_pcb *tpcb, struct pbuf *p, err_t err) {
    if (!p) {
        // Connection closed by server
        printf("Connection closed by server. Received %d bytes.\n", client.bytes_received);
        if (client.bytes_received == DISPLAY_BUFFER_SIZE) {
            client.state = STATE_COMPLETED;
        } else {
            client.state = STATE_FAILED;
        }
        close_client_connection(tpcb);
        return ERR_OK;
    }

    if (err == ERR_OK && p->tot_len > 0) {
        tcp_recved(tpcb, p->tot_len);

        struct pbuf *q;
        for (q = p; q != NULL; q = q->next) {
            uint8_t *payload = (uint8_t *)q->payload;
            int len = q->len;

            for (int i = 0; i < len; i++) {
                if (!client.headers_done) {
                    // Scan for the double CRLF (\r\n\r\n) indicating end of headers
                    if (client.header_search_idx == 0 && payload[i] == '\r') {
                        client.header_search_idx = 1;
                    } else if (client.header_search_idx == 1 && payload[i] == '\n') {
                        client.header_search_idx = 2;
                    } else if (client.header_search_idx == 2 && payload[i] == '\r') {
                        client.header_search_idx = 3;
                    } else if (client.header_search_idx == 3 && payload[i] == '\n') {
                        client.headers_done = true;
                        client.state = STATE_RECEIVING;
                        printf("HTTP headers skipped. Reading e-ink payload...\n");
                    } else {
                        // Reset pattern matching
                        if (payload[i] == '\r') {
                            client.header_search_idx = 1;
                        } else {
                            client.header_search_idx = 0;
                        }
                    }
                } else {
                    // Headers done, store byte in display buffer
                    if (client.bytes_received < DISPLAY_BUFFER_SIZE) {
                        client.buffer[client.bytes_received++] = payload[i];
                    }
                }
            }
        }
        pbuf_free(p);
    }

    // If buffer is full, we can mark complete and close
    if (client.bytes_received == DISPLAY_BUFFER_SIZE) {
        printf("Successfully read all %d bytes of e-ink stream!\n", DISPLAY_BUFFER_SIZE);
        client.state = STATE_COMPLETED;
        close_client_connection(tpcb);
    }

    return ERR_OK;
}

static err_t connect_callback(void *arg, struct tcp_pcb *tpcb, err_t err) {
    if (err != ERR_OK) {
        printf("TCP connect failed callback: %d\n", err);
        client.state = STATE_FAILED;
        return err;
    }

    printf("Connected to server! Sending HTTP request...\n");
    client.state = STATE_CONNECTED;

    tcp_recv(tpcb, recv_callback);

    // Prepare HTTP GET request
    char request[256];
    snprintf(request, sizeof(request),
             "GET /display.bin HTTP/1.1\r\n"
             "Host: %s:%s\r\n"
             "Connection: close\r\n\r\n",
             SERVER_IP, SERVER_PORT);

    err_t write_err = tcp_write(tpcb, request, strlen(request), TCP_WRITE_FLAG_COPY);
    if (write_err != ERR_OK) {
        printf("Failed to write request bytes: %d\n", write_err);
        client.state = STATE_FAILED;
        return write_err;
    }

    // Flush send buffer
    tcp_output(tpcb);
    return ERR_OK;
}

static bool fetch_display_buffer(void) {
    // Reset state variables
    memset(&client, 0, sizeof(client));
    client.state = STATE_CONNECTING;
    client.bytes_received = 0;
    client.headers_done = false;
    client.header_search_idx = 0;

    // Parse IP
    ip_addr_t server_ip;
    if (!ipaddr_aton(SERVER_IP, &server_ip)) {
        printf("Invalid server IP address: %s\n", SERVER_IP);
        return false;
    }

    int port = atoi(SERVER_PORT);

    // Create PCB
    client.pcb = tcp_new();
    if (!client.pcb) {
        printf("Failed to create TCP PCB\n");
        return false;
    }

    tcp_arg(client.pcb, &client);
    tcp_err(client.pcb, err_callback);

    printf("Connecting to Go server at %s:%d...\n", SERVER_IP, port);
    err_t conn_err = tcp_connect(client.pcb, &server_ip, port, connect_callback);
    if (conn_err != ERR_OK) {
        printf("Failed to initiate connection: %d\n", conn_err);
        tcp_abort(client.pcb);
        client.pcb = NULL;
        return false;
    }

    // Wait until completed or failed
    uint32_t timeout_ms = 15000; // 15 seconds HTTP timeout
    uint32_t elapsed_ms = 0;
    while (client.state == STATE_CONNECTING || client.state == STATE_CONNECTED || client.state == STATE_RECEIVING) {
        sleep_ms(100);
        elapsed_ms += 100;
        if (elapsed_ms >= timeout_ms) {
            printf("HTTP connection timed out.\n");
            if (client.pcb) {
                close_client_connection(client.pcb);
            }
            return false;
        }
    }

    return (client.state == STATE_COMPLETED);
}

int main() {
    stdio_init_all();
    sleep_ms(2000); // Wait for serial console to connect

    printf("\n=== Pico W E-Ink AI Stats Display ===\n");

    // Initialize e-paper display hardware (GPIO, SPI)
    printf("Initializing e-paper GPIO/SPI...\n");
    DEV_Module_Init();

    // Initialize Wi-Fi
    printf("Initializing CYW43 Wi-Fi chip...\n");
    if (cyw43_arch_init()) {
        printf("CYW43 init failed!\n");
        return -1;
    }

    // Enable station mode
    cyw43_arch_enable_sta_mode();

    // Main refresh loop
    while (1) {
        // 1. Connect to Wi-Fi
        printf("Connecting to Wi-Fi SSID '%s'...\n", WIFI_SSID);
        // Connect with a 15-second timeout
        if (cyw43_arch_wifi_connect_timeout_ms(WIFI_SSID, WIFI_PASSWORD, CYW43_AUTH_WPA2_AES_PSK, 15000)) {
            printf("Wi-Fi connection failed! Will retry next cycle.\n");
        } else {
            printf("Wi-Fi Connected! IP: %s\n", ip4addr_ntoa(netif_ip4_addr(netif_default)));

            // 2. Fetch screen buffer from Go server
            if (fetch_display_buffer()) {
                printf("Drawing stats dashboard to e-Paper...\n");
                
                // Wake up screen, clear, render buffer, sleep
                EPD_2IN9_V2_Init();
                EPD_2IN9_V2_Display(client.buffer);
                EPD_2IN9_V2_Sleep();
                
                printf("Screen refresh complete!\n");
            } else {
                printf("Failed to fetch image buffer from Go server.\n");
            }

            // 3. Disconnect from Wi-Fi to save power
            printf("Disconnecting from Wi-Fi...\n");
            cyw43_arch_wifi_disconnect();
        }

        // 4. Wait for the configured update interval
        printf("Sleeping for %d minutes...\n", REFRESH_INTERVAL_MIN);
        // Sleep in 1-minute blocks to keep watchdog / output alive if needed
        for (int i = 0; i < REFRESH_INTERVAL_MIN; i++) {
            sleep_ms(60000); 
        }
    }

    DEV_Module_Exit();
    return 0;
}
