#ifndef _DEV_CONFIG_H_
#define _DEV_CONFIG_H_

#include <stdint.h>
#include "pico/stdlib.h"
#include "hardware/spi.h"

// Define basic types
#define UBYTE   uint8_t
#define UWORD   uint16_t
#define UDOUBLE uint32_t

// GPIO Pin mappings for SPI1
#define EPD_DC_PIN      8   // Data/Command Selection
#define EPD_CS_PIN      9   // Chip Select (controlled manually)
#define EPD_CLK_PIN     10  // SPI Clock (SPI1 SCK)
#define EPD_MOSI_PIN    11  // SPI Data (SPI1 TX)
#define EPD_RST_PIN     12  // Reset
#define EPD_BUSY_PIN    13  // Busy Pin

// GPIO Write/Read macros
#define DEV_Digital_Write(_pin, _value) gpio_put(_pin, _value)
#define DEV_Digital_Read(_pin)           gpio_get(_pin)

// Microsecond delay
#define DEV_Delay_ms(_ms) sleep_ms(_ms)

// Module operations
void DEV_Module_Init(void);
void DEV_Module_Exit(void);
void DEV_SPI_WriteByte(UBYTE value);
void DEV_SPI_Write_nByte(UBYTE *pData, UDOUBLE Len);

#endif
