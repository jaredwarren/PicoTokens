cmake_minimum_required(VERSION 3.13)

# Pico W has the CYW43439 Wi-Fi chip; required for cyw43_arch / lwip headers
set(PICO_BOARD pico_w CACHE STRING "Board type" FORCE)

# Initialize the Pico SDK import helper
include(pico_sdk_import.cmake)

project(pico_w_client C CXX ASM)

# Initialize the Raspberry Pi Pico SDK
pico_sdk_init()

# Define the build target and its source files
add_executable(pico_w_client
    pico_w_client.c
    waveshare/DEV_Config.c
    waveshare/EPD_2in9_V2.c
)

# Enable extra output files (including the .uf2 file to drag-and-drop to flash)
pico_add_extra_outputs(pico_w_client)

# Detect if a config.h exists, and define HAS_CONFIG_H for compilation
if(EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/config.h")
    target_compile_definitions(pico_w_client PRIVATE HAS_CONFIG_H=1)
endif()

# Set search paths for headers
target_include_directories(pico_w_client PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}
    ${CMAKE_CURRENT_SOURCE_DIR}/waveshare
)

# Link standard libraries and the CYW43 Wi-Fi stack
# Uses the thread-safe background architecture for networking reliability
target_link_libraries(pico_w_client
    pico_stdlib
    hardware_spi
    pico_cyw43_arch_lwip_threadsafe_background
)
