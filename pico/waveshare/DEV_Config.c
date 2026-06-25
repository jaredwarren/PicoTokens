#include "DEV_Config.h"

void DEV_Module_Init(void) {
    // 1. Initialize Control GPIOs
    gpio_init(EPD_DC_PIN);
    gpio_set_dir(EPD_DC_PIN, GPIO_OUT);

    gpio_init(EPD_RST_PIN);
    gpio_set_dir(EPD_RST_PIN, GPIO_OUT);

    gpio_init(EPD_BUSY_PIN);
    gpio_set_dir(EPD_BUSY_PIN, GPIO_IN);

    // 2. Initialize Chip Select Pin
    gpio_init(EPD_CS_PIN);
    gpio_set_dir(EPD_CS_PIN, GPIO_OUT);
    gpio_put(EPD_CS_PIN, 1); // Deselect initially

    // 3. Initialize SPI1 at 4MHz
    spi_init(spi1, 4 * 1000 * 1000);
    
    // Set pin functionalities to SPI1
    gpio_set_function(EPD_CLK_PIN, GPIO_FUNC_SPI);
    gpio_set_function(EPD_MOSI_PIN, GPIO_FUNC_SPI);
}

void DEV_Module_Exit(void) {
    // Put outputs to safe state
    gpio_put(EPD_CS_PIN, 1);
    gpio_put(EPD_DC_PIN, 0);
    gpio_put(EPD_RST_PIN, 0);
}

void DEV_SPI_WriteByte(UBYTE value) {
    // Select chip
    gpio_put(EPD_CS_PIN, 0);
    // Write blocking
    spi_write_blocking(spi1, &value, 1);
    // Deselect chip
    gpio_put(EPD_CS_PIN, 1);
}

void DEV_SPI_Write_nByte(UBYTE *pData, UDOUBLE Len) {
    // Select chip
    gpio_put(EPD_CS_PIN, 0);
    // Write blocking multiple bytes
    spi_write_blocking(spi1, pData, Len);
    // Deselect chip
    gpio_put(EPD_CS_PIN, 1);
}
