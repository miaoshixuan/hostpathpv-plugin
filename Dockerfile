FROM ubuntu:20.04

COPY _output/hostpathcsi /hostpathcsi

ENTRYPOINT ["/hostpathcsi"]
