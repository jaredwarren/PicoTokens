# Pico W E-Ink AI Token Usage Display

An elegant, low-power display system built for the **Raspberry Pi Pico W** and a **Go backend** to monitor real-time AI token usage and daily expenditure on a **Waveshare 2.9" e-Paper display**.

The Go server aggregates costs from Gemini and Anthropic, generates a sleek dark-themed landscape layout, rotates it 90° to match the screen's portrait SRAM buffer, and packs it into a high-contrast 1-bit stream (4,736 bytes). The Pico W connects to Wi-Fi, retrieves the stream via HTTP, writes it to the display via SPI, and goes to sleep.

---

## 🔌 Hardware Wiring Guide

Connect the Waveshare 2.9" e-paper display to the Raspberry Pi Pico W using the following pin layout:

| E-Paper Pin | Pico W GPIO Pin | Physical Pin ID | Description |
| :--- | :--- | :--- | :--- |
| **VCC** | **3V3** | Pin 36 | 3.3V Power Supply |
| **GND** | **GND** | Pin 38 | Ground |
| **DIN (MOSI)** | **GP11** (SPI1 TX) | Pin 15 | SPI Master Out Slave In |
| **CLK (SCK)**| **GP10** (SPI1 SCK)| Pin 14 | SPI Clock Signal |
| **CS** | **GP9** | Pin 12 | Chip Select (Active Low) |
| **DC** | **GP8** | Pin 11 | Data / Command Selection |
| **RST** | **GP12** | Pin 16 | Hardware Reset |
| **BUSY** | **GP13** | Pin 17 | Busy Status Pin (Active High) |

---

## 🚀 Go Server Setup (your Mac)

The Go server is located in `/server`. It queries the AI APIs periodically, caches the metrics, renders the dashboard, and serves the bytes.

### 1. Requirements
Ensure you have Go installed on your Mac (`brew install go`).

### 2. Environment Configuration
Create a `.env` file or export the following variables in your terminal:
```bash
export PORT=8296
export DAILY_BUDGET=10.00         # Your daily spend limit in USD
export GEMINI_API_KEY="AIzaSy..."  # Optional Gemini API key
export ANTHROPIC_API_KEY="sk-ant-admin..." # Needs Admin permissions
```

### 3. Run the Server
Using the root Makefile:
```bash
make run-server
```
The server will boot on `http://localhost:8296`. 

*If no API keys are provided in the environment, the server runs in **Manual/Mock Mode** using sample data.*

---

## 📊 HTTP Endpoints

- **`GET /display.bin`**: Exposes the raw 4,736-byte packed screen stream. This is fetched by the Pico W.
- **`GET /debug.png`**: Serves a standard PNG representation of the landscape UI. Open this in your browser to inspect design, layout, and font sizing.
- **`GET /api/stats`**: Serves the current metrics as a JSON object.
- **`POST /api/update`**: Allows pushing stats manually if you do not have Admin API keys.
  - **Example Request**:
    ```bash
    curl -X POST http://localhost:8296/api/update \
      -H "Content-Type: application/json" \
      -d '{
        "gemini_cost": 1.45,
        "gemini_weekly_cost": 8.75,
        "gemini_input": 120500,
        "gemini_output": 54000,
        "claude_cost": 0.95,
        "claude_weekly_cost": 5.40,
        "claude_input": 23000,
        "claude_output": 8400
      }'
    ```

---

## 🛠️ Pico W Client Firmware Setup

The firmware is located in `/pico` and is built using CMake and the Raspberry Pi Pico SDK.

### 1. Requirements
Install the ARM cross-compiler toolchain and CMake:
```bash
brew install cmake arm-none-eabi-gcc
```

### 2. Set environment path for Pico SDK
Ensure you have the `pico-sdk` cloned and set the path:
```bash
export PICO_SDK_PATH="/path/to/your/pico-sdk"
```

### 3. Set Credentials
Copy the configuration template:
```bash
cp pico/config.h.example pico/config.h
```
Edit `pico/config.h` and update your Wi-Fi SSID, password, and the local IP address of your Mac.

### 4. Compile the Firmware
```bash
make build-pico
```
This generates build files inside `pico/build/`, including the binary file **`pico_w_client.uf2`**.

### 5. Flash the Pico W
1. Press and hold the **BOOTSEL** button on your Pico W.
2. Connect it to your Mac via a USB cable.
3. Release the **BOOTSEL** button once the Pico W mounts as a USB mass storage device named `RPI-RP2`.
4. Drag and drop **`pico/build/pico_w_client.uf2`** onto the `RPI-RP2` volume.
5. The Pico W will reboot and run the firmware!

### 6. Monitor Serial Output
You can verify operation and monitor logs using `screen`:
```bash
screen /dev/tty.usbmodem* 115250
```
*(Press `Ctrl-A` then `\` to exit screen).*
