cmake_minimum_required(VERSION 3.17)
project(xdag_crypto)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_C_FLAGS "-std=gnu11 -O3 -g -Wall -fpic -Wmissing-prototypes -Wno-unused-result")

set(LIBRARY_OUTPUT_PATH ${CMAKE_SOURCE_DIR})

file(GLOB_RECURSE DAGGER_SOURCES
        dfstools/*.cpp
        )
file(GLOB_RECURSE DAGGER_HEADERS
        dfstools/*.h
        )


OPTION(DEBUG "Build the project using debugging code" OFF)
if(DEBUG)
    MESSAGE("Adding Debug flag...")
    SET(CMAKE_BUILD_TYPE Debug)
    MESSAGE("Build type is " ${CMAKE_BUILD_TYPE})
else()
    add_definitions(-DNDEBUG)
endif(DEBUG)

if(APPLE)
    add_library(
            ${PROJECT_NAME} STATIC
            ./wrapper.cpp
            ./wrapper.h
            ${DAGGER_HEADERS}
            ${DAGGER_SOURCES}
    )
else(UNIX)
    add_library(
            ${PROJECT_NAME} STATIC
            ./wrapper.cpp
            ./wrapper.h
            ${DAGGER_HEADERS}
            ${DAGGER_SOURCES}
    )
endif()

set_target_properties(${PROJECT_NAME} PROPERTIES SUFFIX _${CMAKE_SYSTEM_NAME}.a)

target_link_libraries(
        ${PROJECT_NAME}
        PUBLIC
        ${CMAKE_THREAD_LIBS_INIT}
)

target_link_libraries(
        ${PROJECT_NAME}
        PUBLIC
        m
)

