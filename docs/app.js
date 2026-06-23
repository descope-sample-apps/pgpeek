/* pgpeek docs — progressive enhancements
   Degrades gracefully without JS. Vanilla, no dependencies.
   ================================================================ */

(function () {
  'use strict';

  /* ── 1. Mobile nav toggle ─────────────────────────────────────── */
  const hamburger = document.querySelector('.nav__hamburger');
  const mobileNav = document.querySelector('.nav__mobile');

  if (hamburger && mobileNav) {
    hamburger.addEventListener('click', () => {
      const expanded = hamburger.getAttribute('aria-expanded') === 'true';
      hamburger.setAttribute('aria-expanded', String(!expanded));
      mobileNav.classList.toggle('open', !expanded);
      document.body.style.overflow = expanded ? '' : 'hidden';
    });

    // Close on link click
    mobileNav.querySelectorAll('a').forEach(function (a) {
      a.addEventListener('click', function () {
        hamburger.setAttribute('aria-expanded', 'false');
        mobileNav.classList.remove('open');
        document.body.style.overflow = '';
      });
    });

    // Close on Escape
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && mobileNav.classList.contains('open')) {
        hamburger.setAttribute('aria-expanded', 'false');
        mobileNav.classList.remove('open');
        document.body.style.overflow = '';
        hamburger.focus();
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
      e.preventDefault();
      var top = target.getBoundingClientRect().top + window.scrollY;
      var navHeight = parseInt(
        getComputedStyle(document.documentElement).getPropertyValue('--nav-h'),
        10
      ) || 60;
      window.scrollTo({ top: top - navHeight - 16, behavior: 'smooth' });
      // Update URL hash without jumping
      history.pushState(null, '', id);
    });
  });

})();
