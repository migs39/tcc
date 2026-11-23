# Binário auxiliar do OT (D-002, D-003). Depende do libOTe instalado com
# ENABLE_MR e libsodium; aponte CMAKE_PREFIX_PATH para a instalação.
cmake_minimum_required(VERSION 3.15)
project(ot_helper CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(libOTe REQUIRED COMPONENTS sodium mr)

add_executable(ot-helper main.cpp)
target_link_libraries(ot-helper oc::libOTe)
