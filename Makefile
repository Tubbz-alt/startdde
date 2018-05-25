PREFIX = /usr
GOPATH_DIR = gopath
GOPKG_PREFIX = pkg.deepin.io/dde/startdde
GOBUILD = go build -v
ARCH = $(shell uname -m)

ifdef USE_GCCGO
	extra_gccgo_flags = -Os -O2
	ifeq ($(ARCH),sw_64)
		extra_gccgo_flags += -mieee
	endif
	GOBUILD = gccgo_build.pl -p "gio-2.0 gtk+-3.0 gdk-pixbuf-xlib-2.0 x11 xi libpulse-simple alsa gnome-keyring-1 xfixes xcursor" -f "${extra_gccgo_flags}" -l "m"
endif

all: build

prepare:
	@if [ ! -d ${GOPATH_DIR}/src/${GOPKG_PREFIX} ]; then \
		mkdir -p ${GOPATH_DIR}/src/$(dir ${GOPKG_PREFIX}); \
		ln -sf ../../../.. ${GOPATH_DIR}/src/${GOPKG_PREFIX}; \
		fi

auto_launch_json:
ifdef AUTO_LAUNCH_DCC
	jq --slurpfile dcc misc/auto_launch/dcc.json '.+$$dcc' misc/auto_launch/source.json > misc/config/auto_launch.json
else
	cp misc/auto_launch/source.json misc/config/auto_launch.json
endif

startdde:
	env GOPATH="${CURDIR}/${GOPATH_DIR}:${GOPATH}" ${GOBUILD} -o startdde

build: prepare startdde auto_launch_json

install:
	install -Dm755 startdde ${DESTDIR}${PREFIX}/bin/startdde
	mkdir -p ${DESTDIR}${PREFIX}/share/xsessions
	@for i in $(shell ls misc/xsessions/ | grep -E '*.in$$' );do sed 's|@PREFIX@|$(PREFIX)|g' misc/xsessions/$$i > ${DESTDIR}${PREFIX}/share/xsessions/$${i%.in}; done
	install -Dm755 misc/deepin-session ${DESTDIR}${PREFIX}/sbin/deepin-session
	install -Dm644 misc/lightdm.conf ${DESTDIR}${PREFIX}/share/lightdm/lightdm.conf.d/60-deepin.conf
	mkdir -p ${DESTDIR}${PREFIX}/share/startdde/
	cp -f misc/config/* ${DESTDIR}${PREFIX}/share/startdde/

clean:
	-rm -rf ${GOPATH_DIR}
	-rm -f startdde

rebuild: clean build
