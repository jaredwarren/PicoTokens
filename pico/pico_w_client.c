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
#define SERVER_PORT     "8296"
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
    char header_line[128];
    int header_line_len;
} http_client_t;

// Global client structure
static http_client_t client;
static char last_sync_time[64] = "Never";

// 5x7 ASCII Font Table (ascii 32 to 126)
static const uint8_t font5x7[][5] = {
    {0x00, 0x00, 0x00, 0x00, 0x00}, // (space)
    {0x00, 0x00, 0x5f, 0x00, 0x00}, // !
    {0x00, 0x07, 0x00, 0x07, 0x00}, // "
    {0x14, 0x7f, 0x14, 0x7f, 0x14}, // #
    {0x24, 0x2a, 0x7f, 0x2a, 0x12}, // $
    {0x23, 0x13, 0x08, 0x64, 0x62}, // %
    {0x36, 0x49, 0x55, 0x22, 0x50}, // &
    {0x00, 0x05, 0x03, 0x00, 0x00}, // '
    {0x00, 0x1c, 0x22, 0x41, 0x00}, // (
    {0x00, 0x41, 0x22, 0x1c, 0x00}, // )
    {0x14, 0x08, 0x3e, 0x08, 0x14}, // *
    {0x08, 0x08, 0x3e, 0x08, 0x08}, // +
    {0x00, 0x50, 0x30, 0x00, 0x00}, // ,
    {0x08, 0x08, 0x08, 0x08, 0x08}, // -
    {0x00, 0x60, 0x60, 0x00, 0x00}, // .
    {0x20, 0x10, 0x08, 0x04, 0x02}, // /
    {0x3e, 0x51, 0x49, 0x45, 0x3e}, // 0
    {0x00, 0x42, 0x7f, 0x40, 0x00}, // 1
    {0x42, 0x61, 0x51, 0x49, 0x46}, // 2
    {0x21, 0x41, 0x45, 0x4b, 0x31}, // 3
    {0x18, 0x14, 0x12, 0x7f, 0x10}, // 4
    {0x27, 0x45, 0x45, 0x45, 0x39}, // 5
    {0x3c, 0x4a, 0x49, 0x49, 0x30}, // 6
    {0x01, 0x71, 0x09, 0x05, 0x03}, // 7
    {0x36, 0x49, 0x49, 0x49, 0x36}, // 8
    {0x06, 0x49, 0x49, 0x29, 0x1e}, // 9
    {0x00, 0x36, 0x36, 0x00, 0x00}, // :
    {0x00, 0x56, 0x36, 0x00, 0x00}, // ;
    {0x08, 0x14, 0x22, 0x41, 0x00}, // <
    {0x14, 0x14, 0x14, 0x14, 0x14}, // =
    {0x00, 0x41, 0x22, 0x14, 0x08}, // >
    {0x02, 0x01, 0x51, 0x09, 0x06}, // ?
    {0x32, 0x49, 0x79, 0x41, 0x3e}, // @
    {0x7e, 0x11, 0x11, 0x11, 0x7e}, // A
    {0x7f, 0x49, 0x49, 0x49, 0x36}, // B
    {0x3e, 0x41, 0x41, 0x41, 0x22}, // C
    {0x7f, 0x41, 0x41, 0x22, 0x1c}, // D
    {0x7f, 0x49, 0x49, 0x49, 0x41}, // E
    {0x7f, 0x09, 0x09, 0x09, 0x01}, // F
    {0x3e, 0x41, 0x49, 0x49, 0x7a}, // G
    {0x7f, 0x08, 0x08, 0x08, 0x7f}, // H
    {0x00, 0x41, 0x7f, 0x41, 0x00}, // I
    {0x20, 0x40, 0x41, 0x3f, 0x01}, // J
    {0x7f, 0x08, 0x14, 0x22, 0x41}, // K
    {0x7f, 0x40, 0x40, 0x40, 0x40}, // L
    {0x7f, 0x02, 0x0c, 0x02, 0x7f}, // M
    {0x7f, 0x04, 0x08, 0x10, 0x7f}, // N
    {0x3e, 0x41, 0x41, 0x41, 0x3e}, // O
    {0x7f, 0x09, 0x09, 0x09, 0x06}, // P
    {0x3e, 0x41, 0x51, 0x21, 0x5e}, // Q
    {0x7f, 0x09, 0x19, 0x29, 0x46}, // R
    {0x46, 0x49, 0x49, 0x49, 0x31}, // S
    {0x01, 0x01, 0x7f, 0x01, 0x01}, // T
    {0x3f, 0x40, 0x40, 0x40, 0x3f}, // U
    {0x1f, 0x20, 0x40, 0x20, 0x1f}, // V
    {0x3f, 0x40, 0x38, 0x40, 0x3f}, // W
    {0x63, 0x14, 0x08, 0x14, 0x63}, // X
    {0x07, 0x08, 0x70, 0x08, 0x07}, // Y
    {0x61, 0x51, 0x49, 0x45, 0x43}, // Z
    {0x00, 0x7f, 0x41, 0x41, 0x00}, // [
    {0x02, 0x04, 0x08, 0x10, 0x20}, // \
    {0x00, 0x41, 0x41, 0x7f, 0x00}, // ]
    {0x04, 0x02, 0x01, 0x02, 0x04}, // ^
    {0x40, 0x40, 0x40, 0x40, 0x40}, // _
    {0x00, 0x01, 0x02, 0x04, 0x00}, // `
    {0x20, 0x54, 0x54, 0x54, 0x78}, // a
    {0x7f, 0x48, 0x44, 0x44, 0x38}, // b
    {0x38, 0x44, 0x44, 0x44, 0x20}, // c
    {0x38, 0x44, 0x44, 0x48, 0x7f}, // d
    {0x38, 0x54, 0x54, 0x54, 0x18}, // e
    {0x08, 0x7e, 0x09, 0x01, 0x02}, // f
    {0x0c, 0x52, 0x52, 0x52, 0x3e}, // g
    {0x7f, 0x08, 0x04, 0x04, 0x78}, // h
    {0x00, 0x44, 0x7d, 0x40, 0x00}, // i
    {0x20, 0x40, 0x44, 0x3d, 0x00}, // j
    {0x7f, 0x10, 0x28, 0x44, 0x00}, // k
    {0x00, 0x41, 0x7f, 0x40, 0x00}, // l
    {0x7c, 0x04, 0x18, 0x04, 0x78}, // m
    {0x7c, 0x08, 0x04, 0x04, 0x78}, // n
    {0x38, 0x44, 0x44, 0x44, 0x38}, // o
    {0x7c, 0x14, 0x14, 0x14, 0x08}, // p
    {0x08, 0x14, 0x14, 0x18, 0x7c}, // q
    {0x7c, 0x08, 0x04, 0x04, 0x08}, // r
    {0x48, 0x54, 0x54, 0x54, 0x20}, // s
    {0x04, 0x3f, 0x44, 0x40, 0x20}, // t
    {0x3c, 0x40, 0x40, 0x20, 0x7c}, // u
    {0x1c, 0x20, 0x40, 0x20, 0x1c}, // v
    {0x3c, 0x40, 0x30, 0x40, 0x3c}, // w
    {0x44, 0x28, 0x10, 0x28, 0x44}, // x
    {0x0c, 0x50, 0x50, 0x50, 0x3c}, // y
    {0x44, 0x64, 0x54, 0x4c, 0x44}, // z
    {0x00, 0x08, 0x36, 0x41, 0x00}, // {
    {0x00, 0x00, 0x7f, 0x00, 0x00}, // |
    {0x00, 0x41, 0x36, 0x08, 0x00}, // }
    {0x08, 0x0c, 0x08, 0x18, 0x08}  // ~
};

// Draw utilities for 296x128 landscape logical viewport
static void draw_pixel(uint8_t *buf, int x, int y, bool white) {
    if (x < 0 || x >= 296 || y < 0 || y >= 128) return;
    int xp = 127 - y;
    int yp = x;
    int offset = yp * 16 + (xp / 8);
    int bit = 7 - (xp % 8);
    if (white) {
        buf[offset] |= (1 << bit);
    } else {
        buf[offset] &= ~(1 << bit);
    }
}

static void draw_line(uint8_t *buf, int x1, int y1, int x2, int y2, bool black) {
    if (x1 == x2) { // vertical
        if (y1 > y2) { int tmp = y1; y1 = y2; y2 = tmp; }
        for (int y = y1; y <= y2; y++) draw_pixel(buf, x1, y, !black);
    } else if (y1 == y2) { // horizontal
        if (x1 > x2) { int tmp = x1; x1 = x2; x2 = tmp; }
        for (int x = x1; x <= x2; x++) draw_pixel(buf, x, y1, !black);
    }
}

static void draw_rect(uint8_t *buf, int x1, int y1, int x2, int y2, bool black) {
    draw_line(buf, x1, y1, x2, y1, black);
    draw_line(buf, x1, y2, x2, y2, black);
    draw_line(buf, x1, y1, x1, y2, black);
    draw_line(buf, x2, y1, x2, y2, black);
}

static void draw_char(uint8_t *buf, int x, int y, char c, bool black) {
    if (c < 32 || c > 126) c = ' ';
    int char_idx = c - 32;
    for (int col = 0; col < 5; col++) {
        uint8_t line = font5x7[char_idx][col];
        for (int row = 0; row < 7; row++) {
            if (line & (1 << row)) {
                draw_pixel(buf, x + col, y + row, !black);
            }
        }
    }
}

static void draw_string(uint8_t *buf, int x, int y, const char *str, bool black) {
    while (*str) {
        draw_char(buf, x, y, *str, black);
        x += 6;
        str++;
    }
}

// Render "No Connection" warning screen locally
static void render_no_connection_screen(uint8_t *buf) {
    memset(buf, 0xFF, DISPLAY_BUFFER_SIZE); // Clear to white

    // Draw borders
    draw_rect(buf, 5, 5, 290, 122, true);
    draw_rect(buf, 7, 7, 288, 120, true);

    // Draw warning exclamation box
    draw_rect(buf, 138, 15, 157, 34, true);
    draw_line(buf, 147, 19, 147, 26, true);
    draw_line(buf, 148, 19, 148, 26, true);
    draw_pixel(buf, 147, 29, false); // false = black
    draw_pixel(buf, 148, 29, false);
    draw_pixel(buf, 147, 30, false);
    draw_pixel(buf, 148, 30, false);

    // Write centered text lines
    char line1[] = "CONNECTION FAILURE";
    int w1 = strlen(line1) * 6;
    draw_string(buf, (296 - w1)/2, 48, line1, true);

    char line2[] = "Could not sync with local server";
    int w2 = strlen(line2) * 6;
    draw_string(buf, (296 - w2)/2, 68, line2, true);

    char sync_msg[128];
    snprintf(sync_msg, sizeof(sync_msg), "LAST SYNC: %s", last_sync_time);
    int w3 = strlen(sync_msg) * 6;
    draw_string(buf, (296 - w3)/2, 88, sync_msg, true);
}

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
                    // Accumulate header line for parsing
                    if (payload[i] != '\r' && payload[i] != '\n') {
                        if (client.header_line_len < (int)sizeof(client.header_line) - 1) {
                            client.header_line[client.header_line_len++] = payload[i];
                        }
                    } else if (payload[i] == '\n') {
                        client.header_line[client.header_line_len] = '\0';
                        if (client.header_line_len > 0) {
                            if (strncmp(client.header_line, "X-Sync-Time: ", 13) == 0) {
                                strncpy(last_sync_time, client.header_line + 13, sizeof(last_sync_time) - 1);
                                last_sync_time[sizeof(last_sync_time) - 1] = '\0';
                                printf("Parsed sync time from header: %s\n", last_sync_time);
                            }
                        }
                        client.header_line_len = 0;
                    }

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

    // Put display to sleep immediately to protect it from burnout if boot/wifi fails
    EPD_2IN9_V2_Init();
    EPD_2IN9_V2_Sleep();

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
        cyw43_arch_enable_sta_mode();

        // 1. Connect to Wi-Fi
        printf("Connecting to Wi-Fi SSID '%s'...\n", WIFI_SSID);
        // Connect with a 15-second timeout
        bool success = false;
        if (cyw43_arch_wifi_connect_timeout_ms(WIFI_SSID, WIFI_PASSWORD, CYW43_AUTH_WPA2_AES_PSK, 15000)) {
            printf("Wi-Fi connection failed! Will retry next cycle.\n");
        } else {
            printf("Wi-Fi Connected! IP: %s\n", ip4addr_ntoa(netif_ip4_addr(netif_default)));

            // 2. Fetch screen buffer from Go server
            if (fetch_display_buffer()) {
                printf("Drawing stats dashboard to e-Paper...\n");
                
                // Wake up screen, clear, render buffer, sleep
                EPD_2IN9_V2_Init();
                EPD_2IN9_V2_Clear();
                EPD_2IN9_V2_Display(client.buffer);
                EPD_2IN9_V2_Sleep();
                
                printf("Screen refresh complete!\n");
                success = true;
            } else {
                printf("Failed to fetch image buffer from Go server.\n");
            }

            // 3. Disconnect from Wi-Fi to save power
            printf("Disconnecting from Wi-Fi...\n");
            cyw43_arch_disable_sta_mode();
        }

        if (!success) {
            printf("Connection failed. Rendering local error screen to e-Paper...\n");
            render_no_connection_screen(client.buffer);
            EPD_2IN9_V2_Init();
            EPD_2IN9_V2_Clear();
            EPD_2IN9_V2_Display(client.buffer);
            EPD_2IN9_V2_Sleep();
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
