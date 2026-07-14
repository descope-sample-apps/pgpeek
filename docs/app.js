/* pgpeek docs — progressive enhancements
   Degrades gracefully without JS. Vanilla, no dependencies.
   ================================================================ */

(function () {
  'use strict';

  /* ── 1. Mobile nav toggle ─────────────────────────────────────── */
  const hamburger = document.querySelector('.nav__hamburger');
  const mobileNav = document.querySelector('.nav__mobile');

  if (hamburger && mobileNav) {
    const desktopMedia = window.matchMedia('(min-width: 769px)');
    const backgroundRegions = document.querySelectorAll('main, footer');

    function setBackgroundInert(isInert) {
      backgroundRegions.forEach(function (region) {
        region.toggleAttribute('inert', isInert);
      });
    }

    function getMobileNavFocusables() {
      return [hamburger].concat(
        Array.from(mobileNav.querySelectorAll('a[href]'))
      );
    }

    function openMobileNav() {
      hamburger.setAttribute('aria-expanded', 'true');
      mobileNav.classList.add('open');
      document.body.style.overflow = 'hidden';
      setBackgroundInert(true);

      const firstLink = mobileNav.querySelector('a[href]');
      if (firstLink) firstLink.focus();
    }

    function closeMobileNav(restoreFocus) {
      hamburger.setAttribute('aria-expanded', 'false');
      mobileNav.classList.remove('open');
      document.body.style.overflow = '';
      setBackgroundInert(false);

      if (restoreFocus) {
        const focusTarget = desktopMedia.matches
          ? document.querySelector('.nav__logo')
          : hamburger;
        if (focusTarget) focusTarget.focus();
      }
    }

    hamburger.addEventListener('click', () => {
      const expanded = hamburger.getAttribute('aria-expanded') === 'true';
      if (expanded) {
        closeMobileNav(false);
      } else {
        openMobileNav();
      }
    });

    // Close on link click
    mobileNav.querySelectorAll('a').forEach(function (a) {
      a.addEventListener('click', function () {
        closeMobileNav(false);
      });
    });

    // Keep keyboard focus inside the full-screen menu; close on Escape.
    document.addEventListener('keydown', function (e) {
      if (!mobileNav.classList.contains('open')) return;

      if (e.key === 'Escape') {
        closeMobileNav(true);
        return;
      }

      if (e.key !== 'Tab') return;

      const focusables = getMobileNavFocusables();
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      const active = document.activeElement;

      if (e.shiftKey && active === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      } else if (!focusables.includes(active)) {
        e.preventDefault();
        first.focus();
      }
    });

    desktopMedia.addEventListener('change', function (e) {
      if (e.matches && mobileNav.classList.contains('open')) {
        closeMobileNav(true);
      }
    });
  }

  /* ── 2. Scroll-spy ────────────────────────────────────────────── */
  var sections = document.querySelectorAll('section[id]');
  var navLinks = document.querySelectorAll(
    '.nav__links a[href^="#"], .nav__mobile a[href^="#"]'
  );

  if (sections.length && navLinks.length && 'IntersectionObserver' in window) {
    var spyObserver = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) {
            var id = entry.target.id;
            navLinks.forEach(function (link) {
              link.classList.toggle(
                'active',
                link.getAttribute('href') === '#' + id
              );
            });
          }
        });
      },
      { rootMargin: '-20% 0px -65% 0px', threshold: 0 }
    );
    sections.forEach(function (s) { spyObserver.observe(s); });
  }

  /* ── 3. Copy-to-clipboard on code blocks ──────────────────────── */
  if (navigator.clipboard) {
    document.querySelectorAll('.code-wrap').forEach(function (wrap) {
      var btn = wrap.querySelector('.copy-btn');
      var pre = wrap.querySelector('pre');
      if (!btn || !pre) return;

      btn.addEventListener('click', function () {
        // Strip ANSI-style spans, get plain text
        var text = pre.textContent || '';
        navigator.clipboard.writeText(text.trim()).then(function () {
          btn.textContent = 'copied!';
          btn.classList.add('copied');
          setTimeout(function () {
            btn.textContent = 'copy';
            btn.classList.remove('copied');
          }, 2000);
        }).catch(function () { /* clipboard blocked */ });
      });
    });
  }

  /* ── 4. Scroll reveal ─────────────────────────────────────────── */
  if ('IntersectionObserver' in window) {
    var revealObs = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) {
            entry.target.classList.add('revealed');
            revealObs.unobserve(entry.target);
          }
        });
      },
      { threshold: 0.07, rootMargin: '0px 0px -32px 0px' }
    );
    document.querySelectorAll('.reveal').forEach(function (el) {
      revealObs.observe(el);
    });
  } else {
    // Fallback: reveal everything immediately
    document.querySelectorAll('.reveal').forEach(function (el) {
      el.classList.add('revealed');
    });
  }

  /* ── 5. Tab switcher ──────────────────────────────────────────── */
  document.querySelectorAll('.tab-group').forEach(function (group) {
    var buttons = group.querySelectorAll('.tab-btn');
    var panels  = group.querySelectorAll('.tab-panel');

    function activateTab(index) {
      buttons.forEach(function (b, i) {
        var active = i === index;
        b.setAttribute('aria-selected', String(active));
        b.setAttribute('tabindex', active ? '0' : '-1');
      });
      panels.forEach(function (p, i) {
        p.setAttribute('aria-hidden', String(i !== index));
      });
    }

    buttons.forEach(function (btn, i) {
      btn.addEventListener('click', function () { activateTab(i); });

      // Arrow-key navigation inside tab list
      btn.addEventListener('keydown', function (e) {
        var total = buttons.length;
        if (e.key === 'ArrowRight') {
          activateTab((i + 1) % total);
          buttons[(i + 1) % total].focus();
        } else if (e.key === 'ArrowLeft') {
          activateTab((i - 1 + total) % total);
          buttons[(i - 1 + total) % total].focus();
        }
      });
    });

    // Default: first tab active
    activateTab(0);
  });

  /* ── 6. Smooth-scroll anchor links ───────────────────────────── */
  document.querySelectorAll('a[href^="#"]').forEach(function (a) {
    a.addEventListener('click', function (e) {
      var id = a.getAttribute('href');
      if (id === '#') return;
      var target = document.querySelector(id);
      if (!target) return;
      var fromMobileNav = a.closest('.nav__mobile') !== null;
      e.preventDefault();
      var top = target.getBoundingClientRect().top + window.scrollY;
      var navHeight = parseInt(
        getComputedStyle(document.documentElement).getPropertyValue('--nav-h'),
        10
      ) || 60;
      window.scrollTo({ top: top - navHeight - 16, behavior: 'smooth' });
      // Update URL hash without jumping
      history.pushState(null, '', id);

      if (fromMobileNav) {
        var hadTabindex = target.hasAttribute('tabindex');
        if (!hadTabindex) target.setAttribute('tabindex', '-1');
        target.focus({ preventScroll: true });
        if (!hadTabindex) {
          target.addEventListener('blur', function () {
            target.removeAttribute('tabindex');
          }, { once: true });
        }
      }
    });
  });

})();
