Name:       harbour-feed
Summary:    Self-hosted ranked feed reader
Version:    1.0.0
Release:    1
Group:      Qt/Qt
License:    BSD-3-Clause
URL:        https://github.com/anothertijuano/feed
Source0:    %{name}-%{version}.tar.bz2
Requires:   sailfishsilica-qt5 >= 1.0
BuildRequires:  pkgconfig(sailfishapp) >= 1.0
BuildRequires:  pkgconfig(Qt5Core)
BuildRequires:  pkgconfig(Qt5Qml)
BuildRequires:  pkgconfig(Qt5Quick)

%description
A native Sailfish OS client for feed., the self-hosted ranked feed reader.
Browse your personal feed, upvote, downvote and save content, and manage
RSS/Atom/JSON subscriptions.

%prep
%setup -q -n %{name}-%{version}

%build
%qmake5
make %{?_smp_mflags}

%install
%qmake5_install

%files
%defattr(-,root,root,-)
%{_bindir}/%{name}
%{_datadir}/applications/%{name}.desktop
%{_datadir}/%{name}/qml
%{_datadir}/icons/hicolor/86x86/apps/%{name}.png
%{_datadir}/icons/hicolor/108x108/apps/%{name}.png
%{_datadir}/icons/hicolor/128x128/apps/%{name}.png
%{_datadir}/icons/hicolor/172x172/apps/%{name}.png
%{_datadir}/icons/hicolor/256x256/apps/%{name}.png
