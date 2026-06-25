#ifndef __DEBUG_H_
#define __DEBUG_H_

#include <stdio.h>

// Debug macro mapping to printf
#ifdef EPD_DEBUG
#define Debug(...) printf(__VA_ARGS__)
#else
#define Debug(...)
#endif

#endif
